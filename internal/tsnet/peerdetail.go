package tsnet

import (
	"context"
	"net/netip"
	"sync"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

// PeerDetail is the result of a live ping + whois probe against one peer.
// Ping and WhoIs results are independent: either can fail without the
// other, and there is no overall error — the UI renders whatever came
// back and shows an inline error for whichever half failed.
type PeerDetail struct {
	Ping    *ipnstate.PingResult // nil if the ping RPC itself errored (see PingErr)
	PingErr error

	Owner    string // UserProfile.DisplayName, falling back to LoginName; empty if unknown
	Tags     []string
	WhoIsErr error
}

// PeerDetail runs a live ping (the same "disco" ping tailscale ping uses
// by default) and a whois lookup against ip, concurrently.
func (f *Fetcher) PeerDetail(ctx context.Context, ip netip.Addr) PeerDetail {
	var d PeerDetail
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		d.Ping, d.PingErr = f.lc.Ping(ctx, ip, tailcfg.PingDisco)
	}()

	go func() {
		defer wg.Done()
		who, err := f.lc.WhoIs(ctx, ip.String())
		if err != nil {
			d.WhoIsErr = err
			return
		}
		if who.UserProfile != nil {
			d.Owner = who.UserProfile.DisplayName
			if d.Owner == "" {
				d.Owner = who.UserProfile.LoginName
			}
		}
		if who.Node != nil {
			d.Tags = who.Node.Tags
		}
	}()

	wg.Wait()
	return d
}
