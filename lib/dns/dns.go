package dns

import (
	"context"
	"fmt"
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
		Timeout: 2 * time.Second,
		Dialer: &net.Dialer{
			Timeout: 2 * time.Second,
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: 2 * time.Second}
					return d.DialContext(ctx, network, RoaFinder)
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

// resolveNSIPs resolves A/AAAA for NS hostname via RoaFinder (DefaultClient).
// Minimal: no deep CNAME chase, just pick up A/AAAA answers.
func resolveNSIPs(nsName string) ([]net.IP, error) {
	nsName = dns.Fqdn(nsName)
	ips := make([]net.IP, 0, 4)

	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		msg := new(dns.Msg)
		msg.SetQuestion(nsName, qtype)
		rec, _, err := DefaultClient.Exchange(msg, RoaFinder)
		if err != nil {
			continue
		}
		for _, ans := range rec.Answer {
			switch rr := ans.(type) {
			case *dns.A:
				ips = append(ips, rr.A)
			case *dns.AAAA:
				ips = append(ips, rr.AAAA)
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no A/AAAA for NS %s", nsName)
	}

	// dedup
	seen := map[string]struct{}{}
	uniq := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		k := ip.String()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, ip)
	}
	log.Debug().Msgf("directDNS: NS %s IPs: %v", nsName, uniq)
	return uniq, nil
}

func directDNS(addr string) (net.IP, error) {
	addr = dns.Fqdn(addr)
	log.Debug().Msgf("directDNS: resolving %s", addr)

	msg := new(dns.Msg)

	// find NS servers (may be multiple)
	nsNames := make([]string, 0, 4)
	var cnameTarget string
	var zoneFromSOA string

	msg.SetQuestion(addr, dns.TypeNS)
	rec, _, err := DefaultClient.Exchange(msg, RoaFinder)
	if err != nil {
		return nil, fmt.Errorf("query NS failed: %w", err)
	}

	// even if Answer contains NS, Authority may also include useful NS/SOA.
	// Scan it to avoid missing zone apex (SOA) / additional NS.
	for _, ans := range rec.Answer {
		switch a := ans.(type) {
		case *dns.NS:
			nsNames = append(nsNames, a.Ns)
		case *dns.CNAME:
			cnameTarget = a.Target
		}
	}
	for _, ans := range rec.Ns {
		switch a := ans.(type) {
		case *dns.NS:
			nsNames = append(nsNames, a.Ns)
		case *dns.SOA:
			// SOA.Ns is MNAME (not NS list). Use zone apex to query NS.
			zoneFromSOA = a.Hdr.Name
		}
	}

	// follow CNAME once (keeps behavior close to old code)
	if cnameTarget != "" && cnameTarget != addr {
		log.Debug().Msgf("directDNS: follow CNAME %s -> %s", addr, cnameTarget)
		return directDNS(cnameTarget)
	}

	// if we got SOA but not enough NS, try query NS of zone apex
	if zoneFromSOA != "" {
		zmsg := new(dns.Msg)
		zmsg.SetQuestion(dns.Fqdn(zoneFromSOA), dns.TypeNS)
		zrec, _, zerr := DefaultClient.Exchange(zmsg, RoaFinder)
		if zerr == nil {
			for _, ans := range zrec.Answer {
				if rr, ok := ans.(*dns.NS); ok {
					nsNames = append(nsNames, rr.Ns)
				}
			}
		}
	}

	// dedup ns names
	seenNS := map[string]struct{}{}
	uniqNS := make([]string, 0, len(nsNames))
	for _, n := range nsNames {
		if n == "" {
			continue
		}
		if _, ok := seenNS[n]; ok {
			continue
		}
		seenNS[n] = struct{}{}
		uniqNS = append(uniqNS, n)
	}
	nsNames = uniqNS
	log.Debug().Msgf("directDNS: NS list for %s: %v", addr, nsNames)

	if len(nsNames) == 0 {
		return nil, fmt.Errorf("no NS found")
	}

	// query A/AAAA record from ALL NS (and ALL A/AAAA of each NS)
	var lastErr error
	for _, nsName := range nsNames {
		log.Debug().Msgf("directDNS: resolving NS %s", nsName)
		nsIPs, e := resolveNSIPs(nsName)
		if e != nil {
			log.Debug().Msgf("directDNS: NS %s resolve failed: %v", nsName, e)
			lastErr = e
			continue
		}
		for _, nsIP := range nsIPs {
			nsAddr := net.JoinHostPort(nsIP.String(), "53")
			log.Debug().Msgf(
				"directDNS: query %s via NS %s (%s)",
				addr, nsName, nsAddr,
			)
			// A
			msgA := new(dns.Msg)
			msgA.SetQuestion(addr, dns.TypeA)
			recA, _, eA := DefaultClient.Exchange(msgA, nsAddr)
			if eA == nil {
				for _, ans := range recA.Answer {
					if a, ok := ans.(*dns.A); ok {
						log.Debug().Msgf(
							"directDNS: success %s via %s -> %s",
							addr, nsAddr, a.A.String(),
						)

						return a.A, nil
					}
				}
				log.Debug().Msgf(
					"directDNS: A no-answer %s via %s rcode=%s",
					addr, nsAddr, dns.RcodeToString[recA.Rcode],
				)
				lastErr = fmt.Errorf(
					"A no-answer %s via %s rcode=%s",
					addr, nsAddr, dns.RcodeToString[recA.Rcode],
				)
			} else {
				log.Debug().Msgf(
					"directDNS: A query failed %s via %s: %v",
					addr, nsAddr, eA,
				)
				lastErr = eA
			}

			// AAAA
			msgAAAA := new(dns.Msg)
			msgAAAA.SetQuestion(addr, dns.TypeAAAA)
			recAAAA, _, eAAAA := DefaultClient.Exchange(msgAAAA, nsAddr)
			if eAAAA == nil {
				for _, ans := range recAAAA.Answer {
					if a, ok := ans.(*dns.AAAA); ok {
						return a.AAAA, nil
					}
				}
			} else {
				lastErr = eAAAA
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no record found")
}
