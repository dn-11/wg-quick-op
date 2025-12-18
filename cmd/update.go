package cmd

import (
	"archive/tar"
	"bufio"
	"bytes"
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
	oldPath string
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
			fmt.Printf("Latest: v%s, current: v%s, source: %s\n", latest, cur, used)
			return nil
		}

		if !updateForce && cur != "" && latest != "" && cur == latest {
			fmt.Printf("Already latest: v%s\n", latest)
			return nil
		}

		assetName := expectedAssetName()

		//release.json确认asset存在,取url
		assetURL, err := setAssetURL(rel, used, assetName)
		if err != nil {
			return err
		}

		// checksums内存解析
		sumName := checksumAssetName(latest)
		sumURL, err := setAssetURL(rel, used, sumName)
		if err != nil {
			return err
		}

		sumBytes, err := downloadToBytes(sumURL, ctxTimeout, 2<<20) // 2MB上限
		if err != nil {
			return err
		}
		expected, err := expectedSHAFromChecksumsBytes(sumBytes, assetName)
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

		// 备份旧文件
		if err := renameOverwrite(target, p.oldPath); err != nil {
			return fmt.Errorf("backup old binary failed: %w", err)
		}

		// 流式下载tar.gz -> gzip -> tar,直接覆盖写到target,同时tee做sha256校验
		if err := streamExtractVerifyTarGzToPath(assetURL, target, expected, ctxTimeout); err != nil {
			_ = restoreOld(target, p.oldPath)
			return err
		}

		// 新版本自检
		if err := sanityCheckVersion(target, latest); err != nil {
			_ = restoreOld(target, p.oldPath)
			return err
		}

		// 重启服务
		if !updateNoRestart && fileExists("/etc/init.d/wg-quick-op") {
			fmt.Println("Restarting service...")
			if err := exec.Command("/etc/init.d/wg-quick-op", "restart").Run(); err != nil {
				fmt.Println("Restart failed, rolling back...")
				_ = restoreOld(target, p.oldPath)
				if e := exec.Command("/etc/init.d/wg-quick-op", "start").Run(); e != nil {
					fmt.Fprintf(os.Stderr, "start old service failed: %v\n", e)
				}
				return fmt.Errorf("restart failed: %w", err)
			}
		} else if !updateNoRestart {
			fmt.Println("/etc/init.d/wg-quick-op not found , restart manually onegai")
		}

		_ = os.Remove(p.oldPath)
		fmt.Printf("Update done: v%s\n", latest)
		return nil
	},
}

func init() {
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
	if usedFlag == sourceMirror {
		rel, err := fetchLatestReleaseFromMirror(timeout)
		return rel, sourceMirror, err
	}
	if usedFlag == sourceGitHub {
		rel, err := fetchLatestRelease(timeout)
		return rel, sourceGitHub, err
	}

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

func downloadToBytes(url string, timeout time.Duration, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", "wg-quick-op-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download error: %s: %s", resp.Status, string(b))
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}
	return b, nil
}

func expectedSHAFromChecksumsBytes(checksums []byte, filename string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(checksums))
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

type progressReader struct {
	r         io.Reader
	total     int64
	done      int64
	lastPrint time.Time
	tty       bool
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		if p.tty && time.Since(p.lastPrint) >= 500*time.Millisecond {
			printProgress(p.done, p.total)
			p.lastPrint = time.Now()
		}
	}
	return n, err
}

func streamExtractVerifyTarGzToPath(url, outPath, expectedSHA string, timeout time.Duration) error {
	if isTTY() {
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

	pr := &progressReader{
		r:         resp.Body,
		total:     resp.ContentLength,
		lastPrint: time.Now().Add(-time.Hour),
		tty:       isTTY(),
	}

	h := sha256.New()
	tee := io.TeeReader(pr, h)

	gzr, err := gzip.NewReader(tee)
	if err != nil {
		return fmt.Errorf("gzip reader failed: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read failed: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "wg-quick-op" {
			continue
		}

		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("open output failed: %w", err)
		}

		if _, err := io.CopyN(f, tr, hdr.Size); err != nil {
			f.Close()
			_ = os.Remove(outPath)
			return fmt.Errorf("extract binary failed: %w", err)
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(outPath)
			return fmt.Errorf("close extracted binary failed: %w", err)
		}
		_ = os.Chmod(outPath, 0755)
		found = true
	}

	if isTTY() {
		printProgress(pr.done, pr.total)
		fmt.Println()
	}
	if !found {
		return fmt.Errorf("binary wg-quick-op not found in archive")
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expectedSHA) {
		_ = os.Remove(outPath)
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA, got)
	}

	return nil
}

func sanityCheckVersion(binPath, latest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, binPath, "version").CombinedOutput()
	s := strings.TrimSpace(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("new binary sanity check timed out")
	}
	if err != nil {
		return fmt.Errorf("new binary sanity check failed: %w (output: %s)", err, s)
	}
	if latest != "" && !strings.Contains(s, latest) {
		return fmt.Errorf("new binary version output mismatch (want contains %q, got: %s)", latest, s)
	}

	// show progress
	if isTTY() && s != "" {
		fmt.Printf("New binary: %s\n", s)
	}
	return nil
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
		oldPath: pfx + ".old",
	}
}

func renameOverwrite(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	_ = os.Remove(dst)
	return os.Rename(src, dst)
}

func restoreOld(target, oldPath string) error {
	if err := os.Rename(oldPath, target); err == nil {
		return nil
	}
	_ = os.Remove(target)
	return os.Rename(oldPath, target)
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
