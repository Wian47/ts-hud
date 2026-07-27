package tsnet

import (
	"context"

	"tailscale.com/ipn"
)

// SetWantRunning brings the Tailscale connection up (running=true) or down
// (running=false).
func (f *Fetcher) SetWantRunning(ctx context.Context, running bool) (*ipn.Prefs, error) {
	return f.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: running},
		WantRunningSet: true,
	})
}

// ListProfiles returns the currently active login profile and every
// profile already authenticated on this device.
func (f *Fetcher) ListProfiles(ctx context.Context) (current ipn.LoginProfile, all []ipn.LoginProfile, err error) {
	return f.lc.ProfileStatus(ctx)
}

// SwitchProfile switches the daemon to an already-authenticated profile.
func (f *Fetcher) SwitchProfile(ctx context.Context, id ipn.ProfileID) error {
	return f.lc.SwitchProfile(ctx, id)
}
