package dns

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/dn-11/wg-quick-op/conf"
	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
)

var (
	publicDNS        []netip.AddrPort
	defaultDNSClient = &dns.Client{
		Timeout: 500 * time.Millisecond,
	}
	ResolveUDPAddr = net.ResolveUDPAddr
)

const MaxCnameDepth = 5

func Init() {
	if !conf.EnhancedDNS.DirectResolver.Enabled {
		return
	}

	// 1. load from config
	for _, str := range conf.EnhancedDNS.DirectResolver.ROAFinder {
		// test port existed
		_, _, err := net.SplitHostPort(str)
		if err != nil {
			str = net.JoinHostPort(str, "53")
		}
		// parse addr port
		addrPort, err := netip.ParseAddrPort(str)
		if err != nil {
			log.Error().Err(err).Str("addr", str).Msgf("cannot parse addr from ROAFinder config")
			continue
		}
		publicDNS = append(publicDNS, addrPort)
	}

	// 2. load from /etc/resolv.conf
	if len(publicDNS) == 0 {
		config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			log.Err(err).Msg("Failed to queryWithRetry /etc/resolv.conf")
		} else {
			for _, str := range config.Servers {
				addrPort, err := netip.ParseAddrPort(net.JoinHostPort(str, "53"))
				if err != nil {
					log.Err(err).Str("addr", str).Msg("cannot parse addr from /etc/resolv.conf")
					continue
				}
				publicDNS = append(publicDNS, addrPort)
			}
		}
	}

	// 3. fallback default dns server
	if len(publicDNS) == 0 {
		log.Warn().Msg("no available DNS servers from config, use default DNS servers")
		publicDNS = []netip.AddrPort{
			netip.MustParseAddrPort("223.5.5.5:53"),
			netip.MustParseAddrPort("119.29.29.29:53"),
		}
	}

	ResolveUDPAddr = ResolveUDPAddrDirect
}

func ResolveUDPAddrDirect(_ string, addr string) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split host port failed: %w", err)
	}

	numPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("parse port failed: %w", err)
	}

	ip, err := resolveHostDirect(host)
	if err != nil {
		return nil, fmt.Errorf("queryWithRetry ip addr failed: %w", err)
	}
	return &net.UDPAddr{IP: net.IP(ip.AsSlice()).To16(), Port: numPort}, nil
}

func resolveHostDirect(addr string) (netip.Addr, error) {
	// check if ip
	parsedAddr, err := netip.ParseAddr(addr)
	if err == nil {
		return parsedAddr, nil
	}

	// queryWithRetry dns in direct mode
	ip, err := directDNS(addr)
	if err != nil {
		log.Warn().Msgf("directDNS failed: %v", err)
		return netip.Addr{}, err
	}
	return ip, nil
}

func directDNS(domain string) (netip.Addr, error) {
	domain, err := unfoldCNAME(dns.Fqdn(domain), MaxCnameDepth)
	if err != nil {
		return netip.Addr{}, err
	}

	for ns := range nsAddrIter(domain) {
		for addr := range queryAAndAAAAAddrIter(domain, []netip.AddrPort{netip.AddrPortFrom(ns, 53)}) {
			return addr, nil
		}
	}
	return netip.Addr{}, errors.New("failed to resolve DNS")
}

func unfoldCNAME(domain string, depth int) (string, error) {
	if depth == 0 {
		return "", errors.New("CNAME is too deep")
	}
	rec, err := queryWithRetryWithList(context.Background(), domain, dns.TypeA, publicDNS)
	if err != nil {
		return "", err
	}
	for _, ans := range rec.Answer {
		if ans.Header().Rrtype == dns.TypeCNAME {
			return unfoldCNAME(ans.(*dns.CNAME).Target, depth-1)
		}
	}
	return domain, nil
}

func nsAddrIter(domain string) func(yield func(addr netip.Addr) bool) {
	// find NS
	var nsRec *dns.Msg

DomainTrim:
	for domain != "" {
		rec, err := queryWithRetryWithList(context.Background(), domain, dns.TypeNS, publicDNS)
		if err != nil {
			return nil
		}

		// check SOA
		for _, rr := range rec.Ns {
			soaRR, ok := rr.(*dns.SOA)
			if ok && len(soaRR.Hdr.Name) < len(domain) {
				domain = soaRR.Hdr.Name
				continue DomainTrim
			}
		}

		for _, rr := range rec.Answer {
			if rr.Header().Rrtype == dns.TypeNS {
				nsRec = rec
				break DomainTrim
			}
		}
		_, after, found := strings.Cut(domain, ".")
		if !found {
			return nil
		}
		domain = after
	}

	if domain == "" {
		log.Error().Msg("cannot find NS server")
	}

	rand.Shuffle(len(nsRec.Answer), func(i, j int) {
		nsRec.Answer[i], nsRec.Answer[j] = nsRec.Answer[j], nsRec.Answer[i]
	})

	return func(yield func(addr netip.Addr) bool) {
		for _, rr := range nsRec.Answer {
			ns, ok := rr.(*dns.NS)
			if !ok {
				log.Warn().Str("name", rr.Header().Name).Msgf("%s is not a NS Record", rr.Header().Name)
				continue
			}
			// check additional
			var hasRelative bool
			for _, rr := range nsRec.Extra {
				if rr.Header().Name != ns.Ns {
					continue
				}
				switch rr := rr.(type) {
				case *dns.A:
					addr, ok := netip.AddrFromSlice(rr.A)
					if !ok {
						log.Warn().Str("rr", rr.String()).Msgf("convert dns response to netip")
					}
					hasRelative = true
					if !yield(addr) {
						return
					}
				case *dns.AAAA:
					addr, ok := netip.AddrFromSlice(rr.AAAA)
					if !ok {
						log.Warn().Str("rr", rr.String()).Msgf("convert dns response to netip")
					}
					hasRelative = true
					if !yield(addr) {
						return
					}
				}
			}

			if hasRelative {
				continue
			}

			for addr := range queryAAndAAAAAddrIter(ns.Ns, publicDNS) {
				if !yield(addr) {
					return
				}
			}
		}
	}
}
