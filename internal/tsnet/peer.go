package tsnet

import (
	"net/netip"
	"strings"

	"tailscale.com/tailcfg"
)

// ConnType describes how the local node currently reaches a peer.
type ConnType int

const (
	ConnUnknown ConnType = iota
	ConnDirect
	ConnDERP
	ConnPeerRelay
)

func (c ConnType) String() string {
	switch c {
	case ConnDirect:
		return "Direct"
	case ConnDERP:
		return "DERP"
	case ConnPeerRelay:
		return "Peer-Relay"
	default:
		return "Unknown"
	}
}

// Peer is the subset of Tailscale peer state ts-hud renders and acts on.
type Peer struct {
	ID         tailcfg.StableNodeID
	HostName   string
	DNSName    string
	OS         string
	IPs        []netip.Addr
	Online     bool
	ConnType   ConnType
	DERPRegion string

	// IsExitNode reports whether this peer is the currently selected
	// exit node for outbound internet traffic.
	IsExitNode bool
	// CanBeExitNode reports whether this peer has advertised itself as
	// (and been approved as) an exit node option.
	CanBeExitNode bool
}

// SSHTarget returns the address ts-hud should pass to the ssh binary,
// preferring the peer's MagicDNS name and falling back to its first
// Tailscale IP, then its bare hostname.
func (p Peer) SSHTarget() string {
	if p.DNSName != "" {
		return strings.TrimSuffix(p.DNSName, ".")
	}
	if len(p.IPs) > 0 {
		return p.IPs[0].String()
	}
	return p.HostName
}

// DisplayName returns the name ts-hud shows for a peer: the short
// label of its Tailscale device name (the same name `tailscale status`
// displays), falling back to its raw OS hostname when no DNS name is
// known. The raw hostname is unreliable for display since two peers
// can report the same OS hostname while having distinct device names.
func (p Peer) DisplayName() string {
	if p.DNSName != "" {
		name := strings.TrimSuffix(p.DNSName, ".")
		if i := strings.IndexByte(name, '.'); i != -1 {
			name = name[:i]
		}
		return name
	}
	return p.HostName
}

// MatchesQuery reports whether the peer matches a case-insensitive
// substring search across display name, OS, and Tailscale IPs. An
// empty query matches everything.
func (p Peer) MatchesQuery(query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(p.DisplayName()), q) {
		return true
	}
	if strings.Contains(strings.ToLower(p.OS), q) {
		return true
	}
	for _, ip := range p.IPs {
		if strings.Contains(ip.String(), q) {
			return true
		}
	}
	return false
}
