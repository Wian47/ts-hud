package tsnet

import (
	"context"
	"fmt"
	"sort"
	"time"

	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/tailcfg"
	"tailscale.com/types/logger"
	"tailscale.com/util/eventbus"
)

// DERPRegion is a Tailscale DERP relay region's latency status as measured
// from the local node.
type DERPRegion struct {
	Code      string
	Name      string
	Latency   time.Duration
	Available bool // whether a latency probe to this region succeeded
	Preferred bool // whether this is the node's current "home" DERP region
}

// NetCheckResult is the outcome of a live network check: per-DERP-region
// latency plus a UDP/NAT connectivity verdict, both read from the same
// netcheck.Report.
type NetCheckResult struct {
	Regions []DERPRegion

	UDP bool // a UDP STUN round trip completed

	// HardNAT and NATKnown describe report.MappingVariesByDestIP: if the
	// STUN-observed NAT mapping varies by destination, UDP hole-punching is
	// unreliable ("hard"/symmetric NAT). NATKnown is false when netcheck
	// couldn't determine this (the opt.Bool was unset) — HardNAT has no
	// meaning in that case.
	HardNAT  bool
	NATKnown bool
}

// NetCheck runs a live network check (the same probing `tailscale netcheck`
// does) and returns per-DERP-region latency, sorted with the fastest
// available region first. It opens its own UDP sockets and sends real STUN
// probes, so it takes real wall-clock time (bounded by ctx) rather than
// being a cheap local status read.
func (f *Fetcher) NetCheck(ctx context.Context) (NetCheckResult, error) {
	dm, err := f.lc.CurrentDERPMap(ctx)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("get DERP map: %w", err)
	}
	if dm == nil || len(dm.Regions) == 0 {
		return NetCheckResult{}, fmt.Errorf("no DERP map available from tailscaled")
	}

	bus := eventbus.New()
	defer bus.Close()

	netMon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("start network monitor: %w", err)
	}
	defer netMon.Close()

	c := &netcheck.Client{
		NetMon: netMon,
		Logf:   logger.Discard,
	}
	if err := c.Standalone(ctx, ":0"); err != nil {
		return NetCheckResult{}, fmt.Errorf("bind netcheck probe socket: %w", err)
	}

	report, err := c.GetReport(ctx, dm, nil)
	if err != nil {
		return NetCheckResult{}, fmt.Errorf("get netcheck report: %w", err)
	}

	return netCheckResultFromReport(dm, report), nil
}

func regionsFromReport(dm *tailcfg.DERPMap, report *netcheck.Report) []DERPRegion {
	regions := make([]DERPRegion, 0, len(dm.Regions))
	for id, r := range dm.Regions {
		latency, available := report.RegionLatency[id]
		regions = append(regions, DERPRegion{
			Code:      r.RegionCode,
			Name:      r.RegionName,
			Latency:   latency,
			Available: available,
			Preferred: id == report.PreferredDERP,
		})
	}

	sort.Slice(regions, func(i, j int) bool {
		a, b := regions[i], regions[j]
		if a.Available != b.Available {
			return a.Available // available regions sort first
		}
		if a.Available {
			return a.Latency < b.Latency
		}
		return a.Code < b.Code
	})

	return regions
}

func netCheckResultFromReport(dm *tailcfg.DERPMap, report *netcheck.Report) NetCheckResult {
	hardNAT, natKnown := report.MappingVariesByDestIP.Get()
	return NetCheckResult{
		Regions:  regionsFromReport(dm, report),
		UDP:      report.UDP,
		HardNAT:  hardNAT,
		NATKnown: natKnown,
	}
}
