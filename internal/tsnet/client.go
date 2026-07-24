package tsnet

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
)

// Fetcher retrieves live Tailscale status, preferring the LocalAPI socket
// and falling back to shelling out to the tailscale CLI when the socket
// is unreachable or the caller lacks permission to use it directly.
type Fetcher struct {
	lc *local.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{lc: &local.Client{}}
}

// Fetch returns the current peer list and the local node's own status.
func (f *Fetcher) Fetch(ctx context.Context) ([]Peer, *Peer, error) {
	status, err := f.lc.Status(ctx)
	if err != nil {
		status, err = statusFromCLI(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch tailscale status: %w", err)
		}
	}
	return peersFromStatus(status)
}

func statusFromCLI(ctx context.Context) (*ipnstate.Status, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status --json: %w", err)
	}
	var status ipnstate.Status
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse tailscale status json: %w", err)
	}
	return &status, nil
}

func peersFromStatus(status *ipnstate.Status) ([]Peer, *Peer, error) {
	if status == nil {
		return nil, nil, fmt.Errorf("tsnet: nil status")
	}

	peers := make([]Peer, 0, len(status.Peer))
	for _, ps := range status.Peer {
		peers = append(peers, fromPeerStatus(ps))
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].DisplayName() < peers[j].DisplayName()
	})

	var self *Peer
	if status.Self != nil {
		s := fromPeerStatus(status.Self)
		self = &s
	}
	return peers, self, nil
}

func fromPeerStatus(ps *ipnstate.PeerStatus) Peer {
	ct, region := connTypeFromPeerStatus(ps)
	return Peer{
		HostName:   ps.HostName,
		DNSName:    ps.DNSName,
		OS:         ps.OS,
		IPs:        ps.TailscaleIPs,
		Online:     ps.Online,
		ConnType:   ct,
		DERPRegion: region,
	}
}

func connTypeFromPeerStatus(ps *ipnstate.PeerStatus) (ConnType, string) {
	if !ps.Online {
		return ConnUnknown, ""
	}
	if ps.PeerRelay != "" {
		return ConnPeerRelay, ""
	}
	if ps.CurAddr != "" {
		return ConnDirect, ""
	}
	if ps.Relay != "" {
		return ConnDERP, ps.Relay
	}
	return ConnUnknown, ""
}
