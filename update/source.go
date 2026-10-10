package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const mirrorBase = "https://mirror.macarons.tech/github/dn-11/wg-quick-op/releases"

// ParseSource validates a raw --source flag value and returns the typed Source.
func ParseSource(s string) (Source, error) {
	switch src := Source(s); src {
	case SourceAuto, SourceMirror, SourceGitHub:
		return src, nil
	default:
		return "", fmt.Errorf("invalid source %q, expected auto|mirror|github", s)
	}
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchReleaseJSON(url string, timeout time.Duration, sourceName string) (*ghRelease, error) {
	client := &http.Client{Timeout: timeout}
	req, err := newGetRequest(url)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s release failed: %w", sourceName, err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, sourceName+" api error"); err != nil {
		return nil, err
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode %s release json failed: %w", sourceName, err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("no tag_name in %s latest release", sourceName)
	}
	return &rel, nil
}

func fetchLatestRelease(timeout time.Duration) (*ghRelease, error) {
	return fetchReleaseJSON("https://api.github.com/repos/dn-11/wg-quick-op/releases/latest", timeout, "github")
}

func fetchLatestReleaseFromMirror(timeout time.Duration) (*ghRelease, error) {
	return fetchReleaseJSON(mirrorBase+"/release_latest.json", timeout, "mirror")
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
