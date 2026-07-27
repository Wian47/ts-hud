package tsnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

func TestPeerDetailCombinesPingAndWhoIs(t *testing.T) {
	fake := &fakeLocalClient{
		pingResult: &ipnstate.PingResult{LatencySeconds: 0.0234, Endpoint: "100.64.0.5:41641"},
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{DisplayName: "Alice Smith", LoginName: "alice@example.com"},
			Node:        &tailcfg.Node{Tags: []string{"tag:server"}},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.PingErr != nil {
		t.Errorf("PingErr = %v, want nil", got.PingErr)
	}
	if got.Ping == nil || got.Ping.Endpoint != "100.64.0.5:41641" {
		t.Errorf("Ping = %+v, want Endpoint 100.64.0.5:41641", got.Ping)
	}
	if got.Owner != "Alice Smith" {
		t.Errorf("Owner = %q, want %q", got.Owner, "Alice Smith")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "tag:server" {
		t.Errorf("Tags = %v, want [tag:server]", got.Tags)
	}
}

func TestPeerDetailFallsBackToLoginNameWhenDisplayNameEmpty(t *testing.T) {
	fake := &fakeLocalClient{
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{LoginName: "alice@example.com"},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.Owner != "alice@example.com" {
		t.Errorf("Owner = %q, want %q", got.Owner, "alice@example.com")
	}
}

func TestPeerDetailPingFailureDoesNotBlockWhoIs(t *testing.T) {
	fake := &fakeLocalClient{
		pingErr: errors.New("ping timeout"),
		whoIsResult: &apitype.WhoIsResponse{
			UserProfile: &tailcfg.UserProfile{DisplayName: "Alice Smith"},
		},
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.PingErr == nil {
		t.Fatal("PingErr = nil, want an error")
	}
	if got.Ping != nil {
		t.Errorf("Ping = %+v, want nil", got.Ping)
	}
	if got.Owner != "Alice Smith" {
		t.Errorf("Owner = %q, want %q (WhoIs should still succeed)", got.Owner, "Alice Smith")
	}
}

func TestPeerDetailWhoIsFailureDoesNotBlockPing(t *testing.T) {
	fake := &fakeLocalClient{
		pingResult: &ipnstate.PingResult{LatencySeconds: 0.01, DERPRegionCode: "jnb"},
		whoIsErr:   errors.New("whois: not found"),
	}
	f := &Fetcher{lc: fake}

	got := f.PeerDetail(context.Background(), netip.MustParseAddr("100.64.0.5"))

	if got.WhoIsErr == nil {
		t.Fatal("WhoIsErr = nil, want an error")
	}
	if got.Owner != "" {
		t.Errorf("Owner = %q, want empty", got.Owner)
	}
	if got.Ping == nil || got.Ping.DERPRegionCode != "jnb" {
		t.Errorf("Ping = %+v, want DERPRegionCode jnb (ping should still succeed)", got.Ping)
	}
}
