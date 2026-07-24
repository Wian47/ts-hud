package tsnet

import (
	"context"
	"errors"
	"testing"
	"time"

	"tailscale.com/net/netcheck"
	"tailscale.com/tailcfg"
)

func TestRegionsFromReportSortsAvailableFirstByLatency(t *testing.T) {
	dm := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {RegionID: 1, RegionCode: "nyc", RegionName: "New York City"},
			2: {RegionID: 2, RegionCode: "fra", RegionName: "Frankfurt"},
			3: {RegionID: 3, RegionCode: "syd", RegionName: "Sydney"},
		},
	}
	report := &netcheck.Report{
		PreferredDERP: 2,
		RegionLatency: map[int]time.Duration{
			1: 80 * time.Millisecond,
			2: 20 * time.Millisecond,
			// syd (3) intentionally missing: unreachable/unavailable.
		},
	}

	got := regionsFromReport(dm, report)

	if len(got) != 3 {
		t.Fatalf("regionsFromReport() len = %d, want 3", len(got))
	}

	wantOrder := []string{"fra", "nyc", "syd"}
	for i, want := range wantOrder {
		if got[i].Code != want {
			t.Errorf("regions[%d].Code = %q, want %q (order: %v)", i, got[i].Code, want, codesOf(got))
		}
	}

	fra := got[0]
	if !fra.Available || fra.Latency != 20*time.Millisecond || !fra.Preferred {
		t.Errorf("fra = %+v, want Available=true Latency=20ms Preferred=true", fra)
	}

	syd := got[2]
	if syd.Available || syd.Preferred {
		t.Errorf("syd = %+v, want Available=false Preferred=false", syd)
	}
}

func codesOf(regions []DERPRegion) []string {
	codes := make([]string, len(regions))
	for i, r := range regions {
		codes[i] = r.Code
	}
	return codes
}

func TestNetCheckPropagatesDERPMapError(t *testing.T) {
	fake := &fakeLocalClient{derpMapErr: errors.New("no DERP map")}
	f := &Fetcher{lc: fake}

	if _, err := f.NetCheck(context.Background()); err == nil {
		t.Fatal("NetCheck() error = nil, want non-nil when CurrentDERPMap fails")
	}
}

func TestNetCheckRejectsEmptyDERPMap(t *testing.T) {
	fake := &fakeLocalClient{derpMap: &tailcfg.DERPMap{}}
	f := &Fetcher{lc: fake}

	if _, err := f.NetCheck(context.Background()); err == nil {
		t.Fatal("NetCheck() error = nil, want non-nil when DERP map has no regions")
	}
}
