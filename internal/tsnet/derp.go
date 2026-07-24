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

// NetCheck runs a live network check (the same probing `tailscale netcheck`
// does) and returns per-DERP-region latency, sorted with the fastest
// available region first. It opens its own UDP sockets and sends real STUN
// probes, so it takes real wall-clock time (bounded by ctx) rather than
// being a cheap local status read.
func (f *Fetcher) NetCheck(ctx context.Context) ([]DERPRegion, error) {
	dm, err := f.lc.CurrentDERPMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("get DERP map: %w", err)
	}
	if dm == nil || len(dm.Regions) == 0 {
		return nil, fmt.Errorf("no DERP map available from tailscaled")
	}

	bus := eventbus.New()
	defer bus.Close()

	netMon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		return nil, fmt.Errorf("start network monitor: %w", err)
	}
	defer netMon.Close()

	c := &netcheck.Client{
		NetMon: netMon,
		Logf:   logger.Discard,
	}
	if err := c.Standalone(ctx, ":0"); err != nil {
		return nil, fmt.Errorf("bind netcheck probe socket: %w", err)
	}

	report, err := c.GetReport(ctx, dm, nil)
	if err != nil {
		return nil, fmt.Errorf("get netcheck report: %w", err)
	}

	return regionsFromReport(dm, report), nil
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
