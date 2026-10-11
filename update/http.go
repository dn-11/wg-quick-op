package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const userAgent = "wg-quick-op-updater"

func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

func checkResponse(resp *http.Response, stage string) error {
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s failed: %s: %s", stage, resp.Status, string(b))
	}
	return nil
}
