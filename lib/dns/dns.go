package dns

import (
	"errors"
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

var RoaFinder []string
var DefaultClient *dns.Client

func Init() {
	if !conf.EnhancedDNS.DirectResolver.Enabled {
		ResolveUDPAddr = net.ResolveUDPAddr
		return
	}
	RoaFinder = conf.EnhancedDNS.DirectResolver.ROAFinder
	if len(RoaFinder) == 0 {
		config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err == nil && len(config.Servers) > 0 {
			RoaFinder = config.Servers
		} else {
			RoaFinder = []string{"223.5.5.5:53", "119.29.29.29:53"}
		}
	}
	for i, addr := range RoaFinder {
		if _, err := netip.ParseAddr(addr); err == nil {
			RoaFinder[i] = net.JoinHostPort(addr, "53")
		}
	}
	DefaultClient = &dns.Client{
		Timeout: 500 * time.Millisecond,
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

	ip, err := directDNS(addr)
	if err != nil {
		log.Warn().Msgf("directDNS failed: %v", err)
		return nil, err
	}
	return ip, nil
}

func directDNS(domain string) (net.IP, error) {
	domain = dns.Fqdn(domain)
	var queryStack []query
	for _, server := range RoaFinder {
		queryStack = append([]query{{
			step:   StepNs,
			domain: domain,
			server: server,
		}}, queryStack...)
	}

	for len(queryStack) > 0 {
		curQuery := queryStack[len(queryStack)-1]
		queryStack = queryStack[:len(queryStack)-1]

		switch curQuery.step {
		case StepNs:
			rec, err := resolve(curQuery.domain, dns.TypeNS, curQuery.server)
			if err != nil {
				if errors.Is(err, utils.ErrUnrecoverable) {
					return nil, errors.Unwrap(err)
				}
				continue
			}
			queryStack = make([]query, 0) // reset query stack
			parseNs(&queryStack, domain, rec)
		case StepCname:
			return directDNS(curQuery.domain)
		case StepNsAddr:
			ip, err := queryAddr(curQuery.domain, curQuery.server)
			if err != nil {
				if errors.Is(err, utils.ErrUnrecoverable) {
					log.Debug().Msgf("%s is %s", curQuery.domain, errors.Unwrap(err).Error())
				}
				continue
			}
			parseNsAddr(&queryStack, domain, ip)
		case StepAddr:
			ips, err := queryAddr(curQuery.domain, curQuery.server)
			if err != nil {
				if errors.Is(err, utils.ErrUnrecoverable) {
					return nil, errors.Unwrap(err)
				}
				continue
			}
			if len(ips) > 0 {
				return ips[0], nil
			}
		}
	}

	return nil, errNoAddr
}
