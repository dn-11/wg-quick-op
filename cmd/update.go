package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type updateSource string

const (
	sourceAuto   updateSource = "auto"   // 默认：mirror -> github
	sourceMirror updateSource = "mirror" // 只用镜像，失败就报错
	sourceGitHub updateSource = "github" // 只用官方
)

type updPaths struct {
	newPath, oldPath, tarPath, sumPath string
}

var (
	updateSourceFlag string = string(sourceAuto)
	mirrorBase              = "https://mirror.jp.macaronss.top:8443/github/dn-11/wg-quick-op/releases"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var (
	updateProgress  bool
	updateCheckOnly bool
	updateForce     bool
	updateNoRestart bool
	updateTimeout   time.Duration
)

var updateCmd = &cobra.Command{
	Use:          "update",
	Short:        "Self update wg-quick-op from GitHub Releases",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctxTimeout := updateTimeout
		if ctxTimeout <= 0 {
			ctxTimeout = 120 * time.Second
		}

		// 校验 --source
		usedFlag := updateSource(updateSourceFlag)
		switch usedFlag {
		case sourceAuto, sourceMirror, sourceGitHub:
		default:
			return fmt.Errorf("invalid --source, expected auto|mirror|github")
		}

		rel, used, err := fetchLatestReleaseWithSource(ctxTimeout, usedFlag)
		if err != nil {
			return err
		}
		latest := normalizeVer(rel.TagName)
		cur := normalizeVer(version)

		if updateCheckOnly {
			fmt.Printf(
				"Latest: v%s, current: v%s, source: %s\n",
				latest,
				cur,
				used,
			)
			return nil
		}

		if !updateForce && cur != "" && latest != "" && cur == latest {
			fmt.Printf("Already latest: v%s\n", latest)
			return nil
		}
		assetName := expectedAssetName()

		// 用 release.json 确认asset存在
		assetURL, err := setAssetURL(rel, used, assetName)
		if err != nil {
			return err
		}

		target, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path failed: %w", err)
		}
		target, _ = filepath.EvalSymlinks(target)

		p := buildUpdPaths(target)

		fmt.Printf("Updating to v%s...\n", latest)

		// 下载tar.gz到本地
		defer os.Remove(p.tarPath)

		if err := downloadToFile(assetURL, p.tarPath, ctxTimeout); err != nil {
			return err
		}

		// 下载checksums.txt并校验
		sumName := checksumAssetName(latest)

		// 确认存在
		sumURL, err := setAssetURL(rel, used, sumName)
		if err != nil {
			return err
		}

		defer os.Remove(p.sumPath)

		if err := downloadToFile(sumURL, p.sumPath, ctxTimeout); err != nil {
			return err
		}

		expected, err := expectedSHAFromChecksums(p.sumPath, assetName)
		if err != nil {
			return err
		}
		got, err := sha256File(p.tarPath)
		if err != nil {
			return fmt.Errorf("sha256 failed: %w", err)
		}
		if !strings.EqualFold(got, expected) {
			return fmt.Errorf(
				"sha256 mismatch for %s: expected %s, got %s",
				assetName, expected, got,
			)
		}
		// 校验通过后再解包提取二进制到newPath
		if err := extractBinaryFromTarGz(p.tarPath, p.newPath); err != nil {
			return err
		}

		// 验证新版本是否正常运行
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		vc := exec.CommandContext(ctx, p.newPath, "version")
		vc.Stdout = os.Stdout
		vc.Stderr = os.Stderr

		if err := vc.Run(); err != nil {
			_ = os.Remove(p.newPath)

			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("new binary sanity check timed out")
			}
			return fmt.Errorf("new binary sanity check failed: %w", err)
		}

		// 原子替换，回滚兜底
		rollbackNeeded := true
		defer func() {
			if rollbackNeeded {
				// 如果已经把 old 换走但没成功装回，就尽量恢复
				_ = os.Rename(p.oldPath, target)
				_ = os.Remove(p.newPath)
			}
		}()

		_ = os.Remove(p.oldPath) // 清理历史残留，如果有
		if err := os.Rename(target, p.oldPath); err != nil {
			_ = os.Remove(p.newPath)
			return fmt.Errorf("backup old binary failed: %w", err)
		}
		if err := os.Rename(p.newPath, target); err != nil {
			// 装新失败就回滚
			_ = os.Rename(p.oldPath, target)
			return fmt.Errorf("install new binary failed: %w", err)
		}

		// 重启服务
		if !updateNoRestart && fileExists("/etc/init.d/wg-quick-op") {
			fmt.Println("Restarting service...")

			c := exec.Command("/etc/init.d/wg-quick-op", "restart")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				// restart 失败：回滚二进制,尝试恢复旧服务
				fmt.Println("Restart failed, rolling back...")
				_ = os.Rename(target, p.newPath) // 把坏的挪开
				_ = os.Rename(p.oldPath, target) // 恢复旧版本
				s := exec.Command("/etc/init.d/wg-quick-op", "start")
				s.Stdout = os.Stdout
				s.Stderr = os.Stderr
				rollbackNeeded = false
				_ = s.Run()
				_ = os.Remove(p.newPath)
				return fmt.Errorf("restart failed: %w", err)
			}
		}

		rollbackNeeded = false
		_ = os.Remove(p.oldPath)
		fmt.Printf("Update done: v%s\n", latest)
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateProgress, "progress", true, "Show download progress")
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only check latest version, do not update")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already latest")
	updateCmd.Flags().BoolVar(&updateNoRestart, "no-restart", false, "Do not restart service after updating")
	updateCmd.Flags().DurationVar(&updateTimeout, "timeout", 600*time.Second, "Network timeout")
	updateCmd.Flags().StringVar(
		&updateSourceFlag,
		"source",
		string(sourceAuto),
		`Update source:
		auto    : mirror -> github
		mirror  : https://mirror.jp.macaronss.top:8443/github/dn-11/wg-quick-op/releases
		github  : https://api.github.com/repos/dn-11/wg-quick-op/releases`,
	)

	rootCmd.AddCommand(updateCmd)
}

func fetchLatestRelease(timeout time.Duration) (*ghRelease, error) {
	url := "https://api.github.com/repos/dn-11/wg-quick-op/releases/latest"
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", "wg-quick-op-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github api error: %s: %s", resp.Status, string(b))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode github api json failed: %w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("no tag_name in latest release")
	}
	return &rel, nil
}

func fetchLatestReleaseFromMirror(timeout time.Duration) (*ghRelease, error) {
	url := mirrorBase + "/release_latest.json"

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", "wg-quick-op-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch mirror latest release failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("mirror api error: %s: %s", resp.Status, string(b))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode mirror json failed: %w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("no tag_name in mirror latest release")
	}
	return &rel, nil
}

func fetchLatestReleaseWithSource(timeout time.Duration, usedFlag updateSource) (*ghRelease, updateSource, error) {
	// 强制 mirror
	if usedFlag == sourceMirror {
		rel, err := fetchLatestReleaseFromMirror(timeout)
		return rel, sourceMirror, err
	}

	// 强制 github
	if usedFlag == sourceGitHub {
		rel, err := fetchLatestRelease(timeout)
		return rel, sourceGitHub, err
	}

	// auto：先 mirror，再 github
	rel, err := fetchLatestReleaseFromMirror(timeout)
	if err == nil {
		return rel, sourceMirror, nil
	}
	fmt.Fprintf(os.Stderr, "Mirror source failed (%v), fallback to GitHub...\n", err)
	rel, err = fetchLatestRelease(timeout)
	return rel, sourceGitHub, err
}

func expectedAssetName() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "i386"
	case "arm":
		arch = "armv6"
	}
	return fmt.Sprintf("wg-quick-op_Linux_%s.tar.gz", arch)
}

func normalizeVer(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func checksumAssetName(latest string) string {
	return fmt.Sprintf("wg-quick-op_%s_checksums.txt", latest)
}

func downloadToFile(url, outPath string, timeout time.Duration) error {
	if updateProgress {
		fmt.Printf("Downloading: %s\n", filepath.Base(strings.Split(url, "?")[0]))
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", "wg-quick-op-updater")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download error: %s: %s", resp.Status, string(b))
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	lastPrint := time.Now().Add(-time.Hour)

	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("save file failed: %w", werr)
			}
			downloaded += int64(n)

			if updateProgress && time.Since(lastPrint) >= 500*time.Millisecond {
				printProgress(downloaded, total)
				lastPrint = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("download read failed: %w", rerr)
		}
	}

	if updateProgress {
		printProgress(downloaded, total)
		fmt.Println()
	}

	return nil
}

func printProgress(done, total int64) {
	var line string
	if total > 0 {
		percent := float64(done) * 100 / float64(total)
		line = fmt.Sprintf("Downloaded: %d / %d bytes (%.1f%%)", done, total, percent)
	} else {
		line = fmt.Sprintf("Downloaded: %d bytes", done)
	}
	if isTTY() {
		fmt.Printf("%s\r", line)
	} else {
		fmt.Printf("%s\n", line)
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func expectedSHAFromChecksums(checksumsPath, filename string) (string, error) {
	f, err := os.Open(checksumsPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := strings.ToLower(fields[0])
		if len(sha) != 64 {
			continue
		}
		name := filepath.Base(fields[len(fields)-1])
		if name == filename {
			return sha, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum not found for %s", filename)
}

func extractBinaryFromTarGz(tarGzPath, outPath string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("open tar.gz failed: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader failed: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read failed: %w", err)
		}

		if filepath.Base(hdr.Name) != "wg-quick-op" {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("unexpected tar entry type: %v", hdr.Typeflag)
		}

		tmp, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("open temp output failed: %w", err)
		}

		if _, err := io.CopyN(tmp, tr, hdr.Size); err != nil {
			tmp.Close()
			_ = os.Remove(outPath)
			return fmt.Errorf("extract binary failed: %w", err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(outPath)
			return fmt.Errorf("close extracted binary failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("binary wg-quick-op not found in archive")
}

func findAsset(rel *ghRelease, name string) (string, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

func mirrorLatestURL(name string) string {
	return mirrorBase + "/latest/" + name
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func setAssetURL(rel *ghRelease, used updateSource, name string) (string, error) {
	u, ok := findAsset(rel, name)
	if !ok {
		return "", fmt.Errorf("release asset not found: %s", name)
	}
	if used == sourceMirror {
		return mirrorLatestURL(name), nil
	}
	if u == "" {
		return "", fmt.Errorf("asset url empty: %s", name)
	}
	return u, nil
}

func buildUpdPaths(target string) updPaths {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	pfx := filepath.Join(dir, "."+base)
	return updPaths{
		newPath: pfx + ".new",
		oldPath: pfx + ".old",
		tarPath: pfx + ".asset.tar.gz",
		sumPath: pfx + ".checksums.txt",
	}
}
