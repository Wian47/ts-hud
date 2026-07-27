package tsnet

import (
	"context"
	"net/netip"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
	"tailscale.com/types/views"
)

// GetPrefs returns the daemon's current preferences.
func (f *Fetcher) GetPrefs(ctx context.Context) (*ipn.Prefs, error) {
	return f.lc.GetPrefs(ctx)
}

// SetRunSSH toggles whether this node runs a Tailscale SSH server.
func (f *Fetcher) SetRunSSH(ctx context.Context, run bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:     ipn.Prefs{RunSSH: run},
		RunSSHSet: true,
	})
}

// SetShieldsUp toggles whether incoming connections are blocked.
func (f *Fetcher) SetShieldsUp(ctx context.Context, up bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:        ipn.Prefs{ShieldsUp: up},
		ShieldsUpSet: true,
	})
}

// SetAcceptRoutes toggles whether subnet routes advertised by other nodes
// are accepted.
func (f *Fetcher) SetAcceptRoutes(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:       ipn.Prefs{RouteAll: accept},
		RouteAllSet: true,
	})
}

// SetAcceptDNS toggles whether DNS configuration from the admin panel is
// accepted.
func (f *Fetcher) SetAcceptDNS(ctx context.Context, accept bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:      ipn.Prefs{CorpDNS: accept},
		CorpDNSSet: true,
	})
}

// SetAdvertiseExitNode toggles whether this node advertises the two /0
// "exit node" routes, without disturbing any other subnet routes already
// present in AdvertiseRoutes. "Advertise as exit node" isn't its own pref
// field — tailscale set implements it the same way, via
// Prefs.AdvertisesExitNode() and the two all-IPv4/all-IPv6 /0 prefixes.
func (f *Fetcher) SetAdvertiseExitNode(ctx context.Context, advertise bool) (*ipn.Prefs, error) {
	cur, err := f.lc.GetPrefs(ctx)
	if err != nil {
		return nil, err
	}
	if cur.AdvertisesExitNode() == advertise {
		return cur, nil
	}
	routes := tsaddr.FilterPrefixesCopy(views.SliceOf(cur.AdvertiseRoutes), func(p netip.Prefix) bool {
		return p.Bits() != 0
	})
	if advertise {
		routes = append(routes, tsaddr.AllIPv4(), tsaddr.AllIPv6())
	}
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              ipn.Prefs{AdvertiseRoutes: routes},
		AdvertiseRoutesSet: true,
	})
}
