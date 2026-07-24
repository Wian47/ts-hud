package tsnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// fakeLocalClient substitutes for *local.Client in tests that exercise
// EditPrefs-based operations without a real tailscaled socket.
type fakeLocalClient struct {
	editErr    error
	gotMasked  *ipn.MaskedPrefs
	derpMap    *tailcfg.DERPMap
	derpMapErr error
}

func (f *fakeLocalClient) Status(ctx context.Context) (*ipnstate.Status, error) {
	return nil, errors.New("fakeLocalClient: Status not implemented")
}

func (f *fakeLocalClient) EditPrefs(ctx context.Context, mp *ipn.MaskedPrefs) (*ipn.Prefs, error) {
	f.gotMasked = mp
	if f.editErr != nil {
		return nil, f.editErr
	}
	return &mp.Prefs, nil
}

func (f *fakeLocalClient) CurrentDERPMap(ctx context.Context) (*tailcfg.DERPMap, error) {
	return f.derpMap, f.derpMapErr
}

func TestFetcherSetExitNode(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}
	peer := Peer{ID: "nodeid123", HostName: "exit-node"}

	if err := f.SetExitNode(context.Background(), peer); err != nil {
		t.Fatalf("SetExitNode() error = %v", err)
	}
	if fake.gotMasked == nil {
		t.Fatal("EditPrefs was not called")
	}
	if !fake.gotMasked.ExitNodeIDSet || fake.gotMasked.ExitNodeID != peer.ID {
		t.Errorf("gotMasked = %+v, want ExitNodeIDSet=true ExitNodeID=%q", fake.gotMasked, peer.ID)
	}
}

func TestFetcherClearExitNode(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	if err := f.ClearExitNode(context.Background()); err != nil {
		t.Fatalf("ClearExitNode() error = %v", err)
	}
	if fake.gotMasked == nil || !fake.gotMasked.ExitNodeIDSet || fake.gotMasked.ExitNodeID != "" {
		t.Errorf("gotMasked = %+v, want ExitNodeIDSet=true ExitNodeID empty", fake.gotMasked)
	}
}

func TestFetcherSetExitNodeAllowLANAccess(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	if err := f.SetExitNodeAllowLANAccess(context.Background(), true); err != nil {
		t.Fatalf("SetExitNodeAllowLANAccess() error = %v", err)
	}
	if fake.gotMasked == nil || !fake.gotMasked.ExitNodeAllowLANAccessSet || !fake.gotMasked.ExitNodeAllowLANAccess {
		t.Errorf("gotMasked = %+v, want ExitNodeAllowLANAccessSet=true ExitNodeAllowLANAccess=true", fake.gotMasked)
	}
}

func TestFetcherSetExitNodePropagatesError(t *testing.T) {
	fake := &fakeLocalClient{editErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if err := f.SetExitNode(context.Background(), Peer{}); err == nil {
		t.Fatal("SetExitNode() error = nil, want non-nil")
	}
}

func TestPeersFromStatus(t *testing.T) {
	directKey := key.NewNode().Public()
	derpKey := key.NewNode().Public()
	peerRelayKey := key.NewNode().Public()
	offlineKey := key.NewNode().Public()
	exitNodeKey := key.NewNode().Public()

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
			exitNodeKey: {
				HostName:       "exit-node",
				TailscaleIPs:   []netip.Addr{netip.MustParseAddr("100.64.0.6")},
				Online:         true,
				CurAddr:        "100.64.0.6:41641",
				ExitNode:       true,
				ExitNodeOption: true,
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

	if len(peers) != 5 {
		t.Fatalf("len(peers) = %d, want 5", len(peers))
	}

	// peersFromStatus sorts by hostname.
	wantOrder := []string{"derp-node", "direct-node", "exit-node", "offline-node", "relay-node"}
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
	if got := byHost["exit-node"]; !got.IsExitNode || !got.CanBeExitNode {
		t.Errorf("exit-node = %+v, want IsExitNode=true CanBeExitNode=true", got)
	}
	if got := byHost["direct-node"]; got.IsExitNode || got.CanBeExitNode {
		t.Errorf("direct-node = %+v, want IsExitNode=false CanBeExitNode=false", got)
	}
}

func TestPeersFromStatusNilStatus(t *testing.T) {
	if _, _, err := peersFromStatus(nil); err == nil {
		t.Fatal("peersFromStatus(nil) error = nil, want non-nil")
	}
}
