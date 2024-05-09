package dns

import (
	"fmt"
	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"strconv"
)

var RoaFinder string = "223.5.5.5:53"

func Init() {
	if !conf.EnhancedDNS.DirectResolver.Enabled {
		ResolveUDPAddr = net.ResolveUDPAddr
		return
	}
	RoaFinder = conf.EnhancedDNS.DirectResolver.ROAFinder
	if RoaFinder == "" {
		config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err == nil && len(config.Servers) > 0 {
			RoaFinder = config.Servers[0]
		} else {
			RoaFinder = "223.5.5.5:53"
		}
	}
	if _, err := netip.ParseAddr(RoaFinder); err == nil {
		RoaFinder = net.JoinHostPort(RoaFinder, "53")
	}
}

var ResolveUDPAddr = func(network string, addr string) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split host port failed: %w", err)
	}

	numport, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("parse port failed: %w", err)
	}

	ip, err := resolveIPAddr(host)
	if err != nil {
		return nil, fmt.Errorf("resolve ip addr failed: %w", err)
	}
	return &net.UDPAddr{IP: ip, Port: numport}, nil
}

func resolveIPAddr(addr string) (net.IP, error) {
	parsedAddr, err := netip.ParseAddr(addr)
	if err == nil {
		return net.IP(parsedAddr.AsSlice()).To16(), nil
	}

	return directDNS(addr)
}

func directDNS(addr string) (net.IP, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(addr), dns.TypeSOA)

	c := new(dns.Client)
	rec, _, err := c.Exchange(msg, RoaFinder)
	if err != nil {
		return nil, fmt.Errorf("write msg failed: %w", err)
	}

	reply := append(rec.Answer, rec.Ns...)

	if len(reply) == 0 {
		return nil, fmt.Errorf("no SOA record found")
	}

	var NsServer string
	for _, ans := range reply {
		if a, ok := ans.(*dns.SOA); ok {
			NsServer = a.Ns
			break
		}
	}
	if NsServer == "" {
		return nil, fmt.Errorf("no SOA record found")
	}

	for _, ans := range rec.Answer {
		if a, ok := ans.(*dns.CNAME); ok {
			addr = a.Target
			break
		}
	}

	nsAddr := net.JoinHostPort(NsServer, "53")
	msg.SetQuestion(dns.Fqdn(addr), dns.TypeA)
	rec, _, err = c.Exchange(msg, nsAddr)
	if err == nil {
		for _, ans := range rec.Answer {
			if a, ok := ans.(*dns.A); ok {
				return a.A, nil
			}
		}
	}

	msg.SetQuestion(dns.Fqdn(NsServer), dns.TypeAAAA)
	rec, _, err = c.Exchange(msg, nsAddr)
	if err == nil {
		for _, ans := range rec.Answer {
			if a, ok := ans.(*dns.AAAA); ok {
				return a.AAAA, nil
			}
		}
	}

	return nil, fmt.Errorf("no record found")
}
