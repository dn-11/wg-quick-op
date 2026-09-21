package quick

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var strippedInterfaceKeys = map[string]struct{}{
	"address":      struct{}{},
	"mtu":          struct{}{},
	"dns":          struct{}{},
	"table":        struct{}{},
	"preup":        struct{}{},
	"predown":      struct{}{},
	"postup":       struct{}{},
	"postdown":     struct{}{},
	"saveconfig":   struct{}{},
	"wgbin":        struct{}{},
	"addresslabel": struct{}{},
}

// FindConfigFile resolves an interface name or file path to an existing configuration file.
func FindConfigFile(nameOrPath string) (string, error) {
	// If nameOrPath contains path separators or ends with .conf, treat as file path
	if strings.Contains(nameOrPath, "/") || strings.Contains(nameOrPath, string(filepath.Separator)) || strings.HasSuffix(nameOrPath, ".conf") {
		if fi, err := os.Stat(nameOrPath); err == nil && !fi.IsDir() {
			return nameOrPath, nil
		}
		return "", fmt.Errorf("`%s' does not exist", nameOrPath)
	}

	// Otherwise treat as interface name under /etc/wireguard
	etcConf := filepath.Join("/etc/wireguard", nameOrPath+".conf")
	if fi, err := os.Stat(etcConf); err == nil && !fi.IsDir() {
		return etcConf, nil
	}
	return "", fmt.Errorf("`%s' does not exist", etcConf)
}

// Strip reads the wireguard configuration from an interface name or file path
// and outputs a configuration suitable for use with wg(8).
func Strip(nameOrPath string) (string, error) {
	configFile, err := FindConfigFile(nameOrPath)
	if err != nil {
		return "", err
	}

	fi, err := os.Stat(configFile)
	if err != nil {
		return "", err
	}

	if fi.Mode().Perm()&0007 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: `%s' is world accessible\n", configFile)
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}

	return StripConfig(string(content)), nil
}

// StripConfig takes wireguard configuration text and removes wg-quick specific directives
// from the [Interface] section, preserving comments, empty lines, and peer sections.
func StripConfig(content string) string {
	if content == "" {
		return ""
	}

	trimmed := strings.TrimRight(content, "\r\n")
	lines := strings.Split(trimmed, "\n")

	var sb strings.Builder
	isInterfaceSection := false

	for _, line := range lines {
		rawLine := strings.TrimSuffix(line, "\r")

		// Remove comments for key inspection
		stripped, _, _ := strings.Cut(rawLine, "#")
		strippedTrimmed := strings.TrimSpace(stripped)

		if strings.HasPrefix(strippedTrimmed, "[") {
			if strings.HasSuffix(strippedTrimmed, "]") && strings.EqualFold(strings.TrimSpace(strippedTrimmed[1:len(strippedTrimmed)-1]), "Interface") {
				isInterfaceSection = true
			} else {
				isInterfaceSection = false
			}
		}

		if isInterfaceSection {
			key, _, hasEq := strings.Cut(stripped, "=")
			if hasEq {
				key = strings.TrimSpace(key)
				if _, found := strippedInterfaceKeys[strings.ToLower(key)]; found {
					continue
				}
			}
		}

		sb.WriteString(rawLine)
		sb.WriteByte('\n')
	}

	return sb.String()
}
