package dns

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
)

func TestDirectDNS(t *testing.T) {
	testcases := []string{
		"www.baidu.com",
		"www.hdu.edu.cn",
	}
	RoaFinder = "223.5.5.5:53"
	t.Logf("Using %s as RoaFinder", RoaFinder)

	// start of client init
	ResolveUDPAddr = net.ResolveUDPAddr
	if _, err := netip.ParseAddr(RoaFinder); err == nil {
		RoaFinder = net.JoinHostPort(RoaFinder, "53")
	}
	DefaultClient = &dns.Client{
		Dialer: &net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return net.Dial(network, RoaFinder)
				},
			},
		},
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
	RoaFinder = "223.5.5.5:53"
	addr, err := ResolveUDPAddr("", "baidu.com:12345")
	if err != nil {
		t.Errorf("ResolveUDPAddr error:%v", err)
		return
	}
	t.Log(addr)
}
