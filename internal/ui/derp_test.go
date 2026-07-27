package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wian47/ts-hud/internal/tsnet"
)

var derpTestRegions = []tsnet.DERPRegion{
	{Code: "nyc", Name: "New York City", Available: true, Latency: 20 * time.Millisecond},
}

func TestRenderDERPTableShowsLoadingState(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, true, nil, 60)
	if !strings.Contains(got, "checking DERP latency") {
		t.Errorf("renderDERPTable() = %q, want loading message", got)
	}
}

func TestRenderDERPTableShowsError(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, false, errors.New("boom"), 60)
	if !strings.Contains(got, "boom") {
		t.Errorf("renderDERPTable() = %q, want error message", got)
	}
}

func TestRenderDERPTableShowsNoRegions(t *testing.T) {
	got := renderDERPTable(tsnet.NetCheckResult{}, false, nil, 60)
	if !strings.Contains(got, "no DERP regions available") {
		t.Errorf("renderDERPTable() = %q, want no-regions message", got)
	}
}

func TestRenderDERPTableConnectivitySummary(t *testing.T) {
	tests := []struct {
		name   string
		result tsnet.NetCheckResult
		want   string
	}{
		{
			name:   "udp unavailable",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: false},
			want:   "UDP unavailable",
		},
		{
			name:   "udp ok, hard nat",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, HardNAT: true, NATKnown: true},
			want:   "NAT hard",
		},
		{
			name:   "udp ok, easy nat",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, HardNAT: false, NATKnown: true},
			want:   "NAT easy",
		},
		{
			name:   "udp ok, nat unknown",
			result: tsnet.NetCheckResult{Regions: derpTestRegions, UDP: true, NATKnown: false},
			want:   "NAT unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderDERPTable(tt.result, false, nil, 60)
			if !strings.Contains(got, tt.want) {
				t.Errorf("renderDERPTable() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}
