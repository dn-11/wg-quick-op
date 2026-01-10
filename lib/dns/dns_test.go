package dns

import (
	"net"
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
	RoaFinder = []string{"223.5.5.5:53", "119.29.29.29:53"}
	t.Logf("Using %s as RoaFinder", RoaFinder)

	// start of client init
	ResolveUDPAddr = net.ResolveUDPAddr
	for i := range RoaFinder {
		if _, err := netip.ParseAddr(RoaFinder[i]); err == nil {
			RoaFinder[i] = net.JoinHostPort(RoaFinder[i], "53")
		}
	}
	DefaultClient = &dns.Client{
		Timeout: 500 * time.Millisecond,
	}
	// end of client init

	for _, testcase := range testcases {
		t.Logf("Attempting to resolve %s", testcase)
		ip, err := directDNS(testcase)
		if err != nil {
			t.Errorf("directDNS error:%v", err)
			return
		}
		t.Logf("Resolved IP is %s", ip)
	}
}

func TestResolveUDP(t *testing.T) {
	RoaFinder = []string{"223.5.5.5:53", "119.29.29.29:53"}
	addr, err := ResolveUDPAddr("", "baidu.com:12345")
	if err != nil {
		t.Errorf("ResolveUDPAddr error:%v", err)
		return
	}
	t.Log(addr)
}
