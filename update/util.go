package update

import (
	"os"
	"os/exec"
	"strings"
)

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

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
