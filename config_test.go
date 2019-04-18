package wgquick

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var testConfigs = map[string]string{
	"simple": `[Interface]
Address = 10.200.100.8/24
DNS = 10.200.100.1
PrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=

[Peer]
PublicKey = GtL7fZc/bLnqZldpVofMCD6hDjrK28SsdLxevJ+qtKU=
AllowedIPs = 0.0.0.0/0
PresharedKey = /UwcSPg38hW/D9Y3tcS1FOV0K1wuURMbS0sesJEP5ak=
Endpoint = 123.12.12.1:51820
`,
	"sample-2": `[Interface]
Address = 10.192.122.1/24
Address = 10.10.0.1/16
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
ListenPort = 51820
SaveConfig = true

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
AllowedIPs = 10.192.122.3/32, 10.192.124.1/24

[Peer]
PublicKey = TrMvSoP4jYQlY6RIzBgbssQqY3vxI2Pi+y71lOWWXX0=
AllowedIPs = 10.192.122.4/32, 192.168.0.0/16

[Peer]
PublicKey = gN65BkIKy1eCE9pP1wdc8ROUtkHLF2PfAqYdyYBz6EA=
AllowedIPs = 10.10.10.230/32
`,
	"sample-3": `[Interface]
Address = 10.192.122.1/24
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
ListenPort = 51820
Table = 1234
PostUp = ip rule add ipproto tcp dport 22 table 1234
PreDown = ip rule delete ipproto tcp dport 22 table 1234

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`,
}

func TestExampleConfig(t *testing.T) {
	c := &Config{}
	for name, cfg := range testConfigs {
		t.Run(name, func(t *testing.T) {
			err := c.UnmarshalText([]byte(cfg))
			assert.NoError(t, err)
			tt, err := c.MarshalText()
			assert.NoError(t, err)
			t.Logf("Got after remarshaling:\n%s", tt)
			assert.Equal(t, cfg, string(tt))
		})
	}
}
