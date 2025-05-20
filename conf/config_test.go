package conf

import "testing"

func TestParseConfig(t *testing.T) {
	Init("config-sample.toml")
	t.Logf("config: %+v", DDNS)
	t.Logf("config: %+v", StartOnBoot)
	t.Logf("config: %+v", EnhancedDNS)
	t.Logf("config: %+v", Wireguard)
	t.Logf("config: %+v", Log)
}
