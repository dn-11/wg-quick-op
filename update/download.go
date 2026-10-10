package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
		if p.tty && (p.lastPrint.IsZero() || time.Since(p.lastPrint) >= 500*time.Millisecond) {
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
		r:     resp.Body,
		total: resp.ContentLength,
		tty:   isTTY(),
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
			return fmt.Errorf("extract binary failed: %w", err)
		}
		if err := f.Close(); err != nil {
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
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA, got)
	}
	return nil
}
