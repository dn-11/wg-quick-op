package dns

import (
	"context"
	"fmt"
	"github.com/hdu-dn11/wg-quick-op/conf"
	"github.com/hdu-dn11/wg-quick-op/utils"
	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"net"
	"net/netip"
	"strconv"
)

var RoaFinder string
var DefaultClient *dns.Client

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
	DefaultClient = &dns.Client{
		Dialer: &net.Dialer{
			Resolver: &net.Resolver{
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return net.Dial(network, RoaFinder)
				},
			},
		},
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

	var ip net.IP
	if err := <-utils.GoRetry(3, func() error {
		ip, err = directDNS(addr)
		return err
	}); err != nil {
		// fallback
		log.Warn().Msgf("directDNS failed: %v", err)
		ip, err := net.ResolveIPAddr("ip", addr)
		if err != nil {
			return nil, fmt.Errorf("fallback failed: %w", err)
		}
		return ip.IP, nil
	}

	return ip, nil
}

func directDNS(addr string) (net.IP, error) {
	addr = dns.Fqdn(addr)
	msg := new(dns.Msg)
	msg.SetQuestion(addr, dns.TypeCNAME)
	rec, _, err := DefaultClient.Exchange(msg, RoaFinder)
	if err != nil {
		return nil, fmt.Errorf("write msg failed: %w", err)
	}
	for _, ans := range rec.Answer {
		if a, ok := ans.(*dns.CNAME); ok {
			return directDNS(a.Target)
		}
	}

	var NsServer string
	for fa := addr; dns.Split(fa) != nil; {
		msg.SetQuestion(fa, dns.TypeNS)
		rec, _, err := DefaultClient.Exchange(msg, RoaFinder)
		if err != nil {
			return nil, fmt.Errorf("write msg failed: %w", err)
		}

		for _, ans := range rec.Answer {
			switch a := ans.(type) {
			case *dns.SOA:
				NsServer = a.Ns
			case *dns.NS:
				NsServer = a.Ns
			}
		}

		if NsServer != "" {
			break
		}

		fa = fa[dns.Split(fa)[1]:]
	}

	nsAddr := net.JoinHostPort(NsServer, "53")
	msg.SetQuestion(dns.Fqdn(addr), dns.TypeA)
	rec, _, err = DefaultClient.Exchange(msg, nsAddr)
	if err == nil {
		for _, ans := range rec.Answer {
			if a, ok := ans.(*dns.A); ok {
				return a.A, nil
			}
		}
	}

	msg.SetQuestion(dns.Fqdn(addr), dns.TypeAAAA)
	rec, _, err = DefaultClient.Exchange(msg, nsAddr)
	if err == nil {
		for _, ans := range rec.Answer {
			if a, ok := ans.(*dns.AAAA); ok {
				return a.AAAA, nil
			}
		}
	}

	return nil, fmt.Errorf("no record found")
}
