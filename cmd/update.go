package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
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

// 默认指向dn11
var (
	updateRepoOwner = "dn-11"
	updateRepoName  = "wg-quick-op"
	updateRepoFlag  string
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

		if updateRepoFlag != "" {
			parts := strings.Split(updateRepoFlag, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("invalid --repo format, expected owner/name")
			}
			updateRepoOwner = parts[0]
			updateRepoName = parts[1]
		}

		rel, err := fetchLatestRelease(ctxTimeout)
		if err != nil {
			return err
		}

		latest := normalizeVer(rel.TagName)
		cur := normalizeVer(version)

		if !updateForce && cur != "" && latest != "" && cur == latest {
			fmt.Printf("Already latest: v%s\n", latest)
			return nil
		}

		assetName := expectedAssetName()
		assetURL := ""
		for _, a := range rel.Assets {
			if a.Name == assetName {
				assetURL = a.URL
				break
			}
		}
		if assetURL == "" {
			return fmt.Errorf("release asset not found: %s (tag=%s)", assetName, rel.TagName)
		}

		if updateCheckOnly {
			fmt.Printf("Latest: v%s, current: v%s, asset: %s\n", latest, cur, assetName)
			return nil
		}

		target, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path failed: %w", err)
		}
		target, _ = filepath.EvalSymlinks(target)

		dir := filepath.Dir(target)
		base := filepath.Base(target)
		newPath := filepath.Join(dir, "."+base+".new")
		oldPath := filepath.Join(dir, "."+base+".old")

		fmt.Printf("Updating to v%s...\n", latest)

		// 下载tar.gz到本地
		tarPath := filepath.Join(dir, "."+base+".asset.tar.gz")

		defer os.Remove(tarPath)

		if err := downloadToFile(assetURL, tarPath, ctxTimeout); err != nil {
			return err
		}

		// 下载checksums.txt并校验
		sumName := checksumAssetName(latest)
		sumURL := ""
		for _, a := range rel.Assets {
			if a.Name == sumName {
				sumURL = a.URL
				break
			}
		}
		if sumURL == "" {
			return fmt.Errorf("checksums asset not found: %s", sumName)
		}

		sumPath := filepath.Join(dir, "."+base+".checksums.txt")
		defer os.Remove(sumPath)

		if err := downloadToFile(sumURL, sumPath, ctxTimeout); err != nil {
			return err
		}

		expected, err := expectedSHAFromChecksums(sumPath, assetName)
		if err != nil {
			return err
		}
		got, err := sha256File(tarPath)
		if err != nil {
			return fmt.Errorf("sha256 failed: %w", err)
		}
		if strings.ToLower(got) != strings.ToLower(expected) {
			return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", assetName, expected, got)
		}

		// 校验通过后再解包提取二进制到newPath
		if err := extractBinaryFromTarGz(tarPath, newPath); err != nil {
			return err
		}

		// 验证新版本是否正常运行
		vc := exec.Command(newPath, "version")
		vc.Stdout = os.Stdout
		vc.Stderr = os.Stderr
		if err := vc.Run(); err != nil {
			_ = os.Remove(newPath)
			return fmt.Errorf("new binary sanity check failed: %w", err)

		}

		// 原子替换，回滚兜底
		rollbackNeeded := true
		defer func() {
			if rollbackNeeded {
				// 如果已经把 old 换走但没成功装回，就尽量恢复
				_ = os.Rename(oldPath, target)
				_ = os.Remove(newPath)
			}
		}()

		_ = os.Remove(oldPath) // 清理历史残留，如果有
		if err := os.Rename(target, oldPath); err != nil {
			_ = os.Remove(newPath)
			return fmt.Errorf("backup old binary failed: %w", err)
		}
		if err := os.Rename(newPath, target); err != nil {
			// 装新失败就回滚
			_ = os.Rename(oldPath, target)
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
				_ = os.Rename(target, newPath) // 把坏的挪开
				_ = os.Rename(oldPath, target) // 恢复旧版本
				s := exec.Command("/etc/init.d/wg-quick-op", "start")
				s.Stdout = os.Stdout
				s.Stderr = os.Stderr
				rollbackNeeded = false
				_ = s.Run()
				_ = os.Remove(newPath)
				return fmt.Errorf("restart failed: %w", err)
			}
		}

		rollbackNeeded = false
		_ = os.Remove(oldPath)

		fmt.Printf("Update done: v%s\n", latest)
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateProgress, "progress", true, "Show download progress")
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Only check latest version, do not update")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if already latest")
	updateCmd.Flags().BoolVar(&updateNoRestart, "no-restart", false, "Do not restart service after updating")
	updateCmd.Flags().DurationVar(&updateTimeout, "timeout", 120*time.Second, "Network timeout")
	updateCmd.Flags().StringVar(
		&updateRepoFlag,
		"repo",
		"",
		"GitHub repo to fetch updates from (owner/name)",
	)

	rootCmd.AddCommand(updateCmd)
}

func fetchLatestRelease(timeout time.Duration) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", updateRepoOwner, updateRepoName)

	client := &http.Client{Timeout: timeout}
	req, _ := http.NewRequest("GET", url, nil)
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

func expectedAssetName() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "i386"
	case "arm":
		armv := os.Getenv("GOARM")
		if armv == "" {
			armv = "6"
		}
		arch = "armv" + armv
	}
	return fmt.Sprintf("%s_Linux_%s.tar.gz", updateRepoName, arch)
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
	return fmt.Sprintf("%s_%s_checksums.txt", updateRepoName, latest)
}

func downloadToFile(url, outPath string, timeout time.Duration) error {
	if updateProgress {
		fmt.Printf("Downloading: %s\n", filepath.Base(strings.Split(url, "?")[0]))
	}
	client := &http.Client{Timeout: timeout}
	req, _ := http.NewRequest("GET", url, nil)
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
		if isTTY() {
			fmt.Println()
		}
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

	tmp, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("open temp output failed: %w", err)
	}
	defer tmp.Close()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read failed: %w", err)
		}
		if filepath.Base(hdr.Name) == "wg-quick-op" {
			if _, err := io.Copy(tmp, tr); err != nil {
				return fmt.Errorf("extract binary failed: %w", err)
			}
			return nil
		}
	}
	_ = os.Remove(outPath)
	return fmt.Errorf("binary wg-quick-op not found in archive")
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
