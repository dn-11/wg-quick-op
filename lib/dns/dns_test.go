package dns

import "testing"

func TestDirectDNS(t *testing.T) {
	RoaFinder = "223.5.5.5:53"
	testcases := []string{
		"www.baidu.com",
	}
	for _, testcase := range testcases {
		ip, err := directDNS(testcase)
		if err != nil {
			t.Errorf("directDNS error:%v", err)
			return
		}
		t.Log(ip)

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
