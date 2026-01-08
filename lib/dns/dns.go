package dns

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/dn-11/wg-quick-op/conf"
	"github.com/dn-11/wg-quick-op/utils"
	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
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
		Timeout: 1 * time.Second,
		Dialer: &net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
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

	numPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("parse port failed: %w", err)
	}

	ip, err := resolveIPAddr(host)
	if err != nil {
		return nil, fmt.Errorf("resolve ip addr failed: %w", err)
	}
	return &net.UDPAddr{IP: ip, Port: numPort}, nil
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
		return nil, fmt.Errorf("resolve ip addr failed: %w", err)
	}

	return ip, nil
}

func directDNS(domain string) (net.IP, error) {
	// get NS server address for the domain
	nsServer, err := getNsServer(domain)
	if err != nil {
		if errors.Is(err, errCNAME) {
			return directDNS(nsServer)
		}
		return nil, err
	}

	// query address from NS server
	ip, err := resolveDomainToIP(domain, nsServer)
	if err == nil {
		return ip, nil
	}

	return nil, err
}

var (
	errCNAME  = errors.New("CNAME found")
	errNoNS   = errors.New("no NS found")
	errNoAddr = errors.New("no address found")
)

// return an IP address of a NS server for the given domain
func getNsServer(domain string) (string, error) {
	domain = dns.Fqdn(domain)
	msg := new(dns.Msg)
	msg.SetQuestion(domain, dns.TypeNS)
	rec, _, err := DefaultClient.Exchange(msg, RoaFinder)
	if err != nil {
		return "", fmt.Errorf("write msg failed: %w", err)
	}

	// if additional section has A/AAAA records, use it directly
	if len(rec.Extra) != 0 {
		var ipRRs []dns.RR
		for _, rr := range rec.Extra {
			switch rr.(type) {
			case *dns.A, *dns.AAAA:
				ipRRs = append(ipRRs, rr)
			}
		}
		if len(ipRRs) != 0 {
			rr := randomRRfromSlice(ipRRs)
			switch a := rr.(type) {
			case *dns.A:
				return a.A.String(), nil
			case *dns.AAAA:
				return a.AAAA.String(), nil
			}
		}
	}

	// otherwise, lookup non-root will get an SOA record in authority section
	if len(rec.Ns) != 0 {
		for _, ans := range rec.Ns {
			switch a := ans.(type) {
			case *dns.SOA:
				return getNsServer(a.Hdr.Name)
			}
		}
	}

	if len(rec.Answer) != 0 {
		ans := rec.Answer[0]
		if ans.Header().Rrtype == dns.TypeCNAME {
			return ans.(*dns.CNAME).Target, errCNAME
		}
		if ans.Header().Rrtype == dns.TypeNS {
			// except RRSIG records after NS records
			cur := len(rec.Answer) - 1
			for cur >= 0 {
				if rec.Answer[cur].Header().Rrtype == dns.TypeNS {
					break
				}
				cur--
			}
			nsRRs := rec.Answer[:cur+1]

			rr := randomRRfromSlice(nsRRs)
			if ns, ok := rr.(*dns.NS); ok {
				ip, err := resolveDomainToIP(ns.Ns, RoaFinder)
				if err != nil {
					return "", fmt.Errorf("resolve ns to ip failed: %w", err)
				}
				return ip.String(), nil
			}
		}
	}

	return "", errNoNS
}

func resolveDomainToIP(domain string, server string) (net.IP, error) {
	// prepare server address
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}

	// random select A or AAAA
	qTypes := []uint16{dns.TypeA, dns.TypeAAAA}
	idx := rand.IntN(len(qTypes))

	// first try
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), qTypes[idx])
	rec, _, err := DefaultClient.Exchange(msg, server)
	if err == nil {
		if len(rec.Answer) != 0 {
			ans := randomRRfromSlice(rec.Answer)
			switch a := ans.(type) {
			case *dns.A:
				return a.A, nil
			case *dns.AAAA:
				return a.AAAA, nil
			}
		}
	}

	// second try with another type
	msg.SetQuestion(dns.Fqdn(domain), qTypes[1-idx])
	rec, _, err = DefaultClient.Exchange(msg, server)
	if err == nil {
		if len(rec.Answer) != 0 {
			ans := randomRRfromSlice(rec.Answer)
			switch a := ans.(type) {
			case *dns.A:
				return a.A, nil
			case *dns.AAAA:
				return a.AAAA, nil
			}
		}
	}

	return nil, errNoAddr
}

func randomRRfromSlice(rrs []dns.RR) dns.RR {
	if len(rrs) == 0 {
		return nil
	}
	idx := rand.IntN(len(rrs))
	return rrs[idx]
}
