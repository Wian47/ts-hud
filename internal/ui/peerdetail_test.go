package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Wian47/ts-hud/internal/tsnet"
	"tailscale.com/ipn/ipnstate"
)

var peerDetailTestTarget = tsnet.Peer{HostName: "bravo", OS: "linux", Online: true}

func TestRenderPeerDetailShowsLoadingState(t *testing.T) {
	got := renderPeerDetail(peerDetailTestTarget, tsnet.PeerDetail{}, true)
	if !strings.Contains(got, "probing") {
		t.Errorf("renderPeerDetail() = %q, want loading message", got)
	}
}

func TestRenderPeerDetailDirectPing(t *testing.T) {
	detail := tsnet.PeerDetail{
		Owner: "Alice Smith",
		Tags:  []string{"tag:server"},
		Ping:  &ipnstate.PingResult{LatencySeconds: 0.0234, Endpoint: "100.64.0.5:41641"},
	}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	for _, want := range []string{"Alice Smith", "tag:server", "23.4ms", "100.64.0.5:41641", "direct"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPeerDetail() = %q, want to contain %q", got, want)
		}
	}
}

func TestRenderPeerDetailDERPPing(t *testing.T) {
	detail := tsnet.PeerDetail{
		Ping: &ipnstate.PingResult{LatencySeconds: 0.05, DERPRegionCode: "jnb"},
	}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	for _, want := range []string{"50.0ms", "DERP jnb"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPeerDetail() = %q, want to contain %q", got, want)
		}
	}
}

func TestRenderPeerDetailPingError(t *testing.T) {
	detail := tsnet.PeerDetail{PingErr: errors.New("ping rpc failed")}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "ping rpc failed") {
		t.Errorf("renderPeerDetail() = %q, want the ping error", got)
	}
}

func TestRenderPeerDetailWhoIsError(t *testing.T) {
	detail := tsnet.PeerDetail{WhoIsErr: errors.New("whois: not found")}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "whois: not found") {
		t.Errorf("renderPeerDetail() = %q, want the whois error", got)
	}
}

func TestRenderPeerDetailNoTagsShowsNone(t *testing.T) {
	detail := tsnet.PeerDetail{Owner: "Alice"}
	got := renderPeerDetail(peerDetailTestTarget, detail, false)
	if !strings.Contains(got, "none") {
		t.Errorf("renderPeerDetail() = %q, want \"none\" for empty tags", got)
	}
}
