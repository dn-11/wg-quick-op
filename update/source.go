package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const mirrorBase = "https://mirror.macarons.tech/github/dn-11/wg-quick-op/releases"

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
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

func fetchLatestReleaseWithSource(timeout time.Duration, usedFlag Source) (*ghRelease, Source, error) {
	if usedFlag == SourceMirror {
		rel, err := fetchLatestReleaseFromMirror(timeout)
		return rel, SourceMirror, err
	}
	if usedFlag == SourceGitHub {
		rel, err := fetchLatestRelease(timeout)
		return rel, SourceGitHub, err
	}

	rel, err := fetchLatestReleaseFromMirror(timeout)
	if err == nil {
		return rel, SourceMirror, nil
	}
	fmt.Fprintf(os.Stderr, "Mirror source failed (%v), fallback to GitHub...\n", err)
	rel, err = fetchLatestRelease(timeout)
	return rel, SourceGitHub, err
}
