package tsnet

import (
	"net/netip"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func TestPeersFromStatus(t *testing.T) {
	directKey := key.NewNode().Public()
	derpKey := key.NewNode().Public()
	peerRelayKey := key.NewNode().Public()
	offlineKey := key.NewNode().Public()

	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			HostName:     "self-node",
			TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")},
			Online:       true,
		},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			directKey: {
				HostName:     "direct-node",
				DNSName:      "direct-node.tailnet-1234.ts.net.",
				OS:           "linux",
				TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.2")},
				Online:       true,
				CurAddr:      "100.64.0.2:41641",
			},
			derpKey: {
				HostName:     "derp-node",
				TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.3")},
				Online:       true,
				Relay:        "fra",
			},
			peerRelayKey: {
				HostName:     "relay-node",
				TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.4")},
				Online:       true,
				PeerRelay:    "100.64.0.9:5555:1",
			},
			offlineKey: {
				HostName:     "offline-node",
				TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.5")},
				Online:       false,
			},
		},
	}

	peers, self, err := peersFromStatus(status)
	if err != nil {
		t.Fatalf("peersFromStatus() error = %v", err)
	}

	if self == nil || self.HostName != "self-node" {
		t.Fatalf("self = %+v, want HostName self-node", self)
	}

	if len(peers) != 4 {
		t.Fatalf("len(peers) = %d, want 4", len(peers))
	}

	// peersFromStatus sorts by hostname.
	wantOrder := []string{"derp-node", "direct-node", "offline-node", "relay-node"}
	for i, p := range peers {
		if p.HostName != wantOrder[i] {
			t.Fatalf("peers[%d].HostName = %q, want %q", i, p.HostName, wantOrder[i])
		}
	}

	byHost := make(map[string]Peer, len(peers))
	for _, p := range peers {
		byHost[p.HostName] = p
	}

	if got := byHost["direct-node"].ConnType; got != ConnDirect {
		t.Errorf("direct-node ConnType = %v, want ConnDirect", got)
	}
	if got := byHost["derp-node"]; got.ConnType != ConnDERP || got.DERPRegion != "fra" {
		t.Errorf("derp-node = %+v, want ConnDERP with region fra", got)
	}
	if got := byHost["relay-node"].ConnType; got != ConnPeerRelay {
		t.Errorf("relay-node ConnType = %v, want ConnPeerRelay", got)
	}
	if got := byHost["offline-node"]; got.Online || got.ConnType != ConnUnknown {
		t.Errorf("offline-node = %+v, want Online=false ConnType=ConnUnknown", got)
	}
}

func TestPeersFromStatusNilStatus(t *testing.T) {
	if _, _, err := peersFromStatus(nil); err == nil {
		t.Fatal("peersFromStatus(nil) error = nil, want non-nil")
	}
}
