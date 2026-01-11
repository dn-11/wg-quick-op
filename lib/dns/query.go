package dns

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/dn-11/wg-quick-op/utils"
	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

var globalRateLimiter = rate.NewLimiter(rate.Every(time.Millisecond*20), 1)

// server is an address with port but not only domain name
func queryWithRetry(ctx context.Context, domain string, qType uint16, server netip.AddrPort) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(domain, qType)
	var rec *dns.Msg

	// only retry on exchange error, not for response error
	err := <-utils.GoRetryCtx(ctx, 3, 50*time.Millisecond, func(ctx context.Context) (err error) {
		if err := globalRateLimiter.Wait(ctx); err != nil {
			return err
		}
		rec, _, err = defaultDNSClient.ExchangeContext(ctx, msg, server.String())
		if err != nil {
			log.Warn().Str("domain", domain).Err(err).Str("server", server.String()).Msg("DNS lookup failed")
			return err
		}
		if rec.Rcode != dns.RcodeSuccess {
			log.Warn().Msgf("dns server %s failure with rcode %d", server, rec.Rcode)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func queryWithRetryWithList(ctx context.Context, domain string, qType uint16, dnsList []netip.AddrPort) (*dns.Msg, error) {
	for _, s := range dnsList {
		msg, err := queryWithRetry(ctx, domain, qType, s)
		if err != nil {
			log.Warn().Err(err).Str("domain", domain).Str("server", s.String()).Msg("failed to resolve")
		}
		return msg, nil
	}
	return nil, errors.New("failed to resolve with all public server")
}

func queryAAndAAAAAddrIter(domain string, dnsList []netip.AddrPort) func(yield func(addr netip.Addr) bool) {
	return func(yield func(addr netip.Addr) bool) {
		var (
			wg         sync.WaitGroup
			resultChan = make(chan *dns.Msg, 2)
		)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		wg.Go(func() {
			rec, err := queryWithRetryWithList(ctx, domain, dns.TypeA, dnsList)
			if err != nil {
				log.Err(err).Msgf("DNS query failed: %v", err)
				return
			}
			resultChan <- rec
		})

		wg.Go(func() {
			rec, err := queryWithRetryWithList(ctx, domain, dns.TypeAAAA, dnsList)
			if err != nil {
				log.Err(err).Msgf("DNS query failed: %v", err)
				return
			}
			resultChan <- rec
		})

		go func() {
			wg.Wait()
			close(resultChan)
		}()

		select {
		case <-ctx.Done():
			return
		case result, ok := <-resultChan:
			if !ok {
				return
			}
			for _, rr := range result.Answer {
				switch rr := rr.(type) {
				case *dns.A:
					addr, ok := netip.AddrFromSlice(rr.A)
					if !ok {
						log.Warn().Str("rr", rr.String()).Msgf("convert dns response to netip")
					}
					if !yield(addr) {
						return
					}
				case *dns.AAAA:
					addr, ok := netip.AddrFromSlice(rr.AAAA)
					if !ok {
						log.Warn().Str("rr", rr.String()).Msgf("convert dns response to netip")
					}
					if !yield(addr) {
						return
					}
				}
			}
		}
	}
}
