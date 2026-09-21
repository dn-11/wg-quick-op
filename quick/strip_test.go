package quick

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripConfig(t *testing.T) {
	input := `# Top comment
[Interface]
# Interface comment
Address = 10.200.100.8/24, 10.200.100.9/24
DNS = 10.200.100.1
PrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=
ListenPort = 51820
MTU = 1420
Table = 1234
PreUp = echo preup
PostUp = echo postup
PreDown = echo predown
PostDown = echo postdown
SaveConfig = true
WgBin = /usr/bin/wireguard-go
FwMark = 0x1234

[Peer]
# Peer comment
PublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=
AllowedIPs = 0.0.0.0/0
Endpoint = 123.12.12.1:51820
PersistentKeepalive = 25
`

	expected := `# Top comment
[Interface]
# Interface comment
PrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=
ListenPort = 51820
FwMark = 0x1234

[Peer]
# Peer comment
PublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=
AllowedIPs = 0.0.0.0/0
Endpoint = 123.12.12.1:51820
PersistentKeepalive = 25
`

	actual := StripConfig(input)
	assert.Equal(t, expected, actual)
}

func TestStripConfig_CaseInsensitiveAndComments(t *testing.T) {
	input := `[interface] # lowercase section header
# Address = 1.2.3.4 (commented out, should be kept)
address = 10.0.0.1/24
dns = 1.1.1.1
mtu = 1500
table = off
preup = echo 1
postup = echo 2
predown = echo 3
postdown = echo 4
saveconfig = false
privatekey = mykey= # inline comment
listenport = 12345

[peer]
PublicKey = peerkey=
AllowedIPs = 10.0.0.2/32
`

	expected := `[interface] # lowercase section header
# Address = 1.2.3.4 (commented out, should be kept)
privatekey = mykey= # inline comment
listenport = 12345

[peer]
PublicKey = peerkey=
AllowedIPs = 10.0.0.2/32
`

	actual := StripConfig(input)
	assert.Equal(t, expected, actual)
}

func TestStripConfig_Empty(t *testing.T) {
	assert.Equal(t, "", StripConfig(""))
}

func TestStripConfig_CRLF(t *testing.T) {
	crlfInput := "[Interface]\r\nAddress = 10.0.0.1/24\r\nPrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\r\nListenPort = 51820\r\n\r\n[Peer]\r\nPublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=\r\nAllowedIPs = 0.0.0.0/0\r\n"
	expected := "[Interface]\nPrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\nListenPort = 51820\n\n[Peer]\nPublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=\nAllowedIPs = 0.0.0.0/0\n"
	assert.Equal(t, expected, StripConfig(crlfInput))
}

func TestFindConfigFileAndStrip(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "wgtest0.conf")
	content := `[Interface]
Address = 10.0.0.1/24
PrivateKey = testprivatekey=
ListenPort = 51820

[Peer]
PublicKey = testpublickey=
AllowedIPs = 0.0.0.0/0
`
	err := os.WriteFile(confPath, []byte(content), 0600)
	require.NoError(t, err)

	// Direct file path
	found, err := FindConfigFile(confPath)
	require.NoError(t, err)
	assert.Equal(t, confPath, found)

	// Strip by file path
	stripped, err := Strip(confPath)
	require.NoError(t, err)
	expected := `[Interface]
PrivateKey = testprivatekey=
ListenPort = 51820

[Peer]
PublicKey = testpublickey=
AllowedIPs = 0.0.0.0/0
`
	assert.Equal(t, expected, stripped)

	// Non-existent file path
	nonexistentPath := filepath.Join(tmpDir, "nonexistent.conf")
	_, err = FindConfigFile(nonexistentPath)
	assert.Error(t, err)
	assert.Equal(t, fmt.Sprintf("`%s' does not exist", nonexistentPath), err.Error())

	// Non-existent relative path
	_, err = FindConfigFile("./nonexistent.conf")
	assert.Error(t, err)
	assert.Equal(t, "`./nonexistent.conf' does not exist", err.Error())

	// Non-existent filename ending in .conf
	_, err = FindConfigFile("nonexistent.conf")
	assert.Error(t, err)
	assert.Equal(t, "`nonexistent.conf' does not exist", err.Error())

	// Non-existent interface name (should point to /etc/wireguard/<name>.conf)
	_, err = FindConfigFile("nonexistent_iface")
	assert.Error(t, err)
	expectedEtcConf := filepath.Join("/etc/wireguard", "nonexistent_iface.conf")
	assert.Equal(t, fmt.Sprintf("`%s' does not exist", expectedEtcConf), err.Error())
}

func TestStripMatchesOriginalWgQuick(t *testing.T) {
	wgQuickPath, err := exec.LookPath("wg-quick")
	if err != nil {
		t.Skip("wg-quick not found in PATH, skipping parity test")
	}

	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "wg0.conf")
	content := `# Complex WireGuard config
[Interface]
# Interface level comment
Address = 10.0.0.1/24, 10.0.0.2/24
DNS = 1.1.1.1, 8.8.8.8
MTU = 1420
Table = 51820
PreUp = echo preup
PostUp = echo postup
PreDown = echo predown
PostDown = echo postdown
SaveConfig = true
PrivateKey = YAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
ListenPort = 51820
FwMark = 0x51820

[Peer]
# Peer 1 comment
PublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=
AllowedIPs = 0.0.0.0/0
Endpoint = 1.2.3.4:51820
PersistentKeepalive = 25

[Peer]
# Peer 2 comment
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
AllowedIPs = 192.168.1.0/24
`
	err = os.WriteFile(confPath, []byte(content), 0600)
	require.NoError(t, err)

	cmd := exec.Command(wgQuickPath, "strip", confPath)
	out, err := cmd.Output()
	require.NoError(t, err)

	strippedGo := StripConfig(content)
	// Normalize any trailing extra newlines from bash echo
	assert.Equal(t, strings.TrimRight(string(out), "\n"), strings.TrimRight(strippedGo, "\n"))
}

