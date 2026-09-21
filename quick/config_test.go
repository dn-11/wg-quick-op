package quick

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
FwMark = 51820
MTU = 1380
Table = 1234
WgBin = wireguard-go
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

// 验证省略、显式零值和 off 在解析与序列化时保持不同语义。
func TestOptionalInterfaceSettings(t *testing.T) {
	const privateKey = "PrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\n"
	tests := []struct {
		name       string
		directives string
		mtu        *int
		table      *Table
		tableID    int
		fwmark     *int
		wantErr    bool
	}{
		{name: "omitted"},
		{name: "explicit MTU zero", directives: "MTU = 0\n", mtu: new(0)},
		{name: "table auto", directives: "Table = auto\n", table: new(tableAuto)},
		{name: "custom table", directives: "Table = 1234\n", table: new(Table(1234)), tableID: 1234},
		{name: "table off", directives: "Table = off\n", table: new(tableOff), tableID: -1},
		{name: "table zero", directives: "Table = 0\n", wantErr: true},
		{name: "fwmark off", directives: "FwMark = off\n", fwmark: new(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "[Interface]\n" + privateKey + tt.directives
			cfg := &Config{}

			err := cfg.UnmarshalText([]byte(input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.mtu, cfg.MTU)
			assert.Equal(t, tt.table, cfg.Table)
			assert.Equal(t, tt.tableID, cfg.Table.ID())
			assert.Equal(t, tt.fwmark, cfg.FirewallMark)

			output, err := cfg.MarshalText()
			assert.NoError(t, err)
			assert.Equal(t, input, string(output))
		})
	}
}

func TestFwMark(t *testing.T) {
	// Test FwMark hex
	c := &Config{}
	err := c.UnmarshalText([]byte("[Interface]\nPrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\nFwMark = 0x1234\n"))
	assert.NoError(t, err)
	assert.NotNil(t, c.FirewallMark)
	assert.Equal(t, 0x1234, *c.FirewallMark)

	// Test FwMark off sets FirewallMark to 0 to clear mark
	c = &Config{}
	err = c.UnmarshalText([]byte("[Interface]\nPrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\nFwMark = off\n"))
	assert.NoError(t, err)
	assert.NotNil(t, c.FirewallMark)
	assert.Equal(t, 0, *c.FirewallMark)

	// Test FwMark unspecified leaves FirewallMark as nil
	c = &Config{}
	err = c.UnmarshalText([]byte("[Interface]\nPrivateKey = oK56DE9Ue9zK76rAc8pBl6opph+1v36lm7cXXsQKrQM=\n"))
	assert.NoError(t, err)
	assert.Nil(t, c.FirewallMark)
}
