package tsnet

import (
	"net/netip"
	"testing"
)

func TestConnTypeString(t *testing.T) {
	cases := []struct {
		ct   ConnType
		want string
	}{
		{ConnDirect, "Direct"},
		{ConnDERP, "DERP"},
		{ConnPeerRelay, "Peer-Relay"},
		{ConnUnknown, "Unknown"},
	}
	for _, c := range cases {
		if got := c.ct.String(); got != c.want {
			t.Errorf("ConnType(%d).String() = %q, want %q", c.ct, got, c.want)
		}
	}
}

func TestPeerSSHTarget(t *testing.T) {
	ip := netip.MustParseAddr("100.64.0.5")

	tests := []struct {
		name string
		peer Peer
		want string
	}{
		{
			name: "prefers DNS name without trailing dot",
			peer: Peer{DNSName: "workstation.tailnet-1234.ts.net.", IPs: []netip.Addr{ip}},
			want: "workstation.tailnet-1234.ts.net",
		},
		{
			name: "falls back to first IP when DNS name is empty",
			peer: Peer{DNSName: "", IPs: []netip.Addr{ip}},
			want: "100.64.0.5",
		},
		{
			name: "falls back to hostname when nothing else is set",
			peer: Peer{HostName: "workstation"},
			want: "workstation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.peer.SSHTarget(); got != tt.want {
				t.Errorf("SSHTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPeerMatchesQuery(t *testing.T) {
	p := Peer{
		HostName: "build-server",
		OS:       "linux",
		IPs:      []netip.Addr{netip.MustParseAddr("100.64.0.9")},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"build", true},
		{"BUILD-SERVER", true},
		{"linux", true},
		{"100.64.0.9", true},
		{"windows", false},
		{"nope", false},
	}

	for _, tt := range tests {
		if got := p.MatchesQuery(tt.query); got != tt.want {
			t.Errorf("MatchesQuery(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestPeerMatchesQueryUsesDisplayName(t *testing.T) {
	// A peer's raw OS hostname can differ from (or collide with) its
	// assigned Tailscale device name. Search should match what's shown.
	p := Peer{HostName: "fedora", DNSName: "acer-swift.tail865ddd.ts.net."}

	if !p.MatchesQuery("acer") {
		t.Error("MatchesQuery(\"acer\") = false, want true (should match display name)")
	}
}

func TestPeerDisplayName(t *testing.T) {
	tests := []struct {
		name string
		peer Peer
		want string
	}{
		{
			name: "uses short label from DNS name when present",
			peer: Peer{HostName: "fedora", DNSName: "acer-swift.tail865ddd.ts.net."},
			want: "acer-swift",
		},
		{
			name: "falls back to hostname when DNS name is empty",
			peer: Peer{HostName: "build-server", DNSName: ""},
			want: "build-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.peer.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
