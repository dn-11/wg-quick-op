package dns

import (
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDirectDNS(t *testing.T) {
	testcases := []string{
		"www.baidu.com",
		"www.hdu.edu.cn",
	}
	publicDNS = []netip.AddrPort{netip.MustParseAddrPort("223.5.5.5:53"), netip.MustParseAddrPort("119.29.29.29:53")}
	defaultDNSClient = &dns.Client{
		Timeout: 500 * time.Millisecond,
	}

	for _, testcase := range testcases {
		t.Logf("Attempting to queryWithRetry %s", testcase)
		ip, err := directDNS(testcase)
		if err != nil {
			t.Errorf("directDNS error:%v", err)
			return
		}
		t.Logf("Resolved IP is %s", ip)
	}
}

func TestResolveUDP(t *testing.T) {
	publicDNS = []netip.AddrPort{netip.MustParseAddrPort("223.5.5.5:53"), netip.MustParseAddrPort("119.29.29.29:53")}
	addr, err := ResolveUDPAddr("", "baidu.com:12345")
	if err != nil {
		t.Errorf("ResolveUDPAddr error:%v", err)
		return
	}
	t.Log(addr)
}
