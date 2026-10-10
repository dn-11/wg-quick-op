package update

import (
	"fmt"
	"os"
)

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
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
