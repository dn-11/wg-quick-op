package update

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func targetAssetName() string {
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

func checksumAssetName(latest string) string {
	return fmt.Sprintf("wg-quick-op_%s_checksums.txt", latest)
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

func setAssetURL(rel *ghRelease, used Source, name string) (string, error) {
	u, ok := findAsset(rel, name)
	if !ok {
		return "", fmt.Errorf("release asset not found: %s", name)
	}
	if used == SourceMirror {
		return mirrorLatestURL(name), nil
	}
	if u == "" {
		return "", fmt.Errorf("release asset has empty url: %s", name)
	}
	return u, nil
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
