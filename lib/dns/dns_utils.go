package dns

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"github.com/dn-11/wg-quick-op/utils"
	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
)

var (
	errCNAME  = errors.New("CNAME found")
	errDomain = errors.New("domain not found")
	errNoAddr = errors.New("no address found")
)

type Step int

const (
	ns Step = iota
	nsAddr
	addr
)

type query struct {
	step   Step
	domain string
	server string
}

// server is an address with port but not only domain name
func resolve(domain string, qType uint16, server string) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(domain, qType)
	var rec *dns.Msg
	var err error
	if err := <-utils.GoRetry(3, 50*time.Millisecond, func() error {
		rec, _, err = DefaultClient.Exchange(msg, server)
		if err != nil {
			log.Debug().Msgf("dns request %s on %s failed: %v", domain, server, err)
			return err
		}
		switch rec.Rcode {
		case dns.RcodeSuccess:
			return nil
		case dns.RcodeNameError:
			return errors.Join(utils.ErrUnrecoverable, errDomain)
		case dns.RcodeServerFailure, dns.RcodeRefused, dns.RcodeNotImplemented:
			return fmt.Errorf("dns server %s failure with rcode %d", server, rec.Rcode)
		default:
			return fmt.Errorf("dns request %s on %s got rcode %d", domain, server, rec.Rcode)
		}
	}); err != nil {
		return nil, err
	}
	return rec, nil
}

func queryAddr(domain string, server string) ([]net.IP, error) {
	type result struct {
		ips []net.IP
		err error
	}
	resultCh := make(chan result, 2)

	go func() {
		var ips []net.IP
		rec, err := resolve(domain, dns.TypeA, server)
		if err != nil {
			resultCh <- result{nil, err}
			return
		}
		for _, ans := range rec.Answer {
			switch a := ans.(type) {
			case *dns.A:
				ips = append(ips, a.A)
			}
		}
		resultCh <- result{ips, nil}
	}()

	go func() {
		var ips []net.IP
		rec, err := resolve(domain, dns.TypeAAAA, server)
		if err != nil {
			resultCh <- result{nil, err}
			return
		}
		for _, ans := range rec.Answer {
			switch a := ans.(type) {
			case *dns.AAAA:
				ips = append(ips, a.AAAA)
			}
		}
		resultCh <- result{ips, nil}
	}()

	var finalIPs []net.IP
	var finalErr error
	for range 2 {
		res := <-resultCh
		if res.err != nil {
			finalErr = res.err
			continue
		}
		finalIPs = append(finalIPs, res.ips...)
	}
	if len(finalIPs) == 0 && finalErr != nil {
		return nil, finalErr
	}
	return finalIPs, nil
}

func parseNs(s *[]query, domain string, rec *dns.Msg) (error, string) {
	var nsDomain []string
	if len(rec.Answer) != 0 {
		ans := rec.Answer[0]
		if ans.Header().Rrtype == dns.TypeCNAME {
			return errCNAME, ans.(*dns.CNAME).Target
		}
		if ans.Header().Rrtype == dns.TypeNS {
			for _, rr := range rec.Answer {
				switch rr := rr.(type) {
				case *dns.NS:
					nsDomain = append(nsDomain, rr.Ns)
					for _, r := range RoaFinder {
						*s = append(*s, query{
							step:   nsAddr,
							domain: rr.Ns,
							server: r,
						})
					}
				}
			}
			rand.Shuffle(len(*s), func(i, j int) {
				(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
			})
		}
	}

	if len(rec.Ns) != 0 {
		for _, ans := range rec.Ns {
			switch a := ans.(type) {
			case *dns.SOA:
				for _, r := range RoaFinder {
					*s = append(*s, query{
						step:   ns,
						domain: a.Hdr.Name,
						server: r,
					})
				}
			}
		}
	}

	// if additional section has A/AAAA records, use it
	if len(rec.Extra) != 0 {
		var as []query
		for _, rr := range rec.Extra {
			switch rr.(type) {
			case *dns.A, *dns.AAAA:
				for _, nsd := range nsDomain {
					if rr.Header().Name != nsd {
						continue
					}

					var ip net.IP
					switch a := rr.(type) {
					case *dns.A:
						ip = a.A
					case *dns.AAAA:
						ip = a.AAAA
					}

					as = append(as, query{
						step:   addr,
						domain: domain,
						server: net.JoinHostPort(ip.String(), "53"),
					})
				}
			}
		}
		rand.Shuffle(len(as), func(i, j int) {
			as[i], as[j] = as[j], as[i]
		})
		*s = append(*s, as...)
	}

	return nil, ""
}

func parseNsAddr(s *[]query, domain string, rec []net.IP) {
	var qs []query
	for _, ip := range rec {
		qs = append(qs, query{
			step:   addr,
			domain: domain,
			server: net.JoinHostPort(ip.String(), "53"),
		})
	}
	rand.Shuffle(len(qs), func(i, j int) {
		qs[i], qs[j] = qs[j], qs[i]
	})
	*s = append(*s, qs...)
}
