package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const (
	mirrorBase      = "https://mirror.macarons.tech/github/dn-11/wg-quick-op/releases"
	githubLatestURL = "https://api.github.com/repos/dn-11/wg-quick-op/releases/latest"
)

// ParseSource validates a raw --source flag value and returns the typed Source.
func ParseSource(s string) (Source, error) {
	switch src := Source(s); src {
	case SourceAuto, SourceMirror, SourceGitHub:
		return src, nil
	default:
		return "", fmt.Errorf("invalid source %q, expected auto|mirror|github", s)
	}
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func fetchReleaseJSON(ctx context.Context, url, sourceName string) (*ghRelease, error) {
	req, err := newGetRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
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

func fetchLatestRelease(ctx context.Context) (*ghRelease, error) {
	return fetchReleaseJSON(ctx, githubLatestURL, "github")
}

func fetchLatestReleaseFromMirror(ctx context.Context) (*ghRelease, error) {
	return fetchReleaseJSON(ctx, mirrorBase+"/release_latest.json", "mirror")
}

func fetchLatestReleaseWithSource(ctx context.Context, usedFlag Source) (*ghRelease, Source, error) {
	if usedFlag == SourceMirror {
		rel, err := fetchLatestReleaseFromMirror(ctx)
		return rel, SourceMirror, err
	}
	if usedFlag == SourceGitHub {
		rel, err := fetchLatestRelease(ctx)
		return rel, SourceGitHub, err
	}

	rel, err := fetchLatestReleaseFromMirror(ctx)
	if err == nil {
		return rel, SourceMirror, nil
	}
	// If ctx is already done, the whole operation is being aborted; don't waste
	// a second request on the fallback source.
	if ctx.Err() != nil {
		return nil, usedFlag, err
	}
	fmt.Fprintf(os.Stderr, "Mirror source failed (%v), fallback to GitHub...\n", err)
	rel, err = fetchLatestRelease(ctx)
	return rel, SourceGitHub, err
}
