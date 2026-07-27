package tsnet

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
)

func TestGetPrefs(t *testing.T) {
	want := &ipn.Prefs{RunSSH: true}
	fake := &fakeLocalClient{getPrefsResult: want}
	f := &Fetcher{lc: fake}

	got, err := f.GetPrefs(context.Background())
	if err != nil {
		t.Fatalf("GetPrefs() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("GetPrefs() = %p, want %p", got, want)
	}
}

func TestSetRunSSH(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	got, err := f.SetRunSSH(context.Background(), true)
	if err != nil {
		t.Fatalf("SetRunSSH() error = %v, want nil", err)
	}
	if !fake.gotMasked.RunSSHSet || !fake.gotMasked.Prefs.RunSSH {
		t.Errorf("gotMasked = %+v, want RunSSHSet=true, RunSSH=true", fake.gotMasked)
	}
	if got == nil || !got.RunSSH {
		t.Errorf("SetRunSSH() = %+v, want RunSSH=true", got)
	}
}

func TestSetShieldsUp(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetShieldsUp(context.Background(), true)
	if err != nil {
		t.Fatalf("SetShieldsUp() error = %v, want nil", err)
	}
	if !fake.gotMasked.ShieldsUpSet || !fake.gotMasked.Prefs.ShieldsUp {
		t.Errorf("gotMasked = %+v, want ShieldsUpSet=true, ShieldsUp=true", fake.gotMasked)
	}
}

func TestSetAcceptRoutes(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetAcceptRoutes(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAcceptRoutes() error = %v, want nil", err)
	}
	if !fake.gotMasked.RouteAllSet || !fake.gotMasked.Prefs.RouteAll {
		t.Errorf("gotMasked = %+v, want RouteAllSet=true, RouteAll=true", fake.gotMasked)
	}
}

func TestSetAcceptDNS(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetAcceptDNS(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAcceptDNS() error = %v, want nil", err)
	}
	if !fake.gotMasked.CorpDNSSet || fake.gotMasked.Prefs.CorpDNS {
		t.Errorf("gotMasked = %+v, want CorpDNSSet=true, CorpDNS=false", fake.gotMasked)
	}
}

func TestSetAdvertiseExitNodeTurnsOnFromEmpty(t *testing.T) {
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{}}
	f := &Fetcher{lc: fake}

	got, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	if !fake.gotMasked.AdvertiseRoutesSet {
		t.Fatal("gotMasked.AdvertiseRoutesSet = false, want true")
	}
	if len(fake.gotMasked.Prefs.AdvertiseRoutes) != 2 {
		t.Fatalf("AdvertiseRoutes = %v, want 2 routes (all-v4, all-v6)", fake.gotMasked.Prefs.AdvertiseRoutes)
	}
	if got == nil || !got.AdvertisesExitNode() {
		t.Errorf("SetAdvertiseExitNode() = %+v, want AdvertisesExitNode() true", got)
	}
}

func TestSetAdvertiseExitNodePreservesSubnetRoutesWhenTurningOn(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/8")
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{AdvertiseRoutes: []netip.Prefix{subnet}}}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	routes := fake.gotMasked.Prefs.AdvertiseRoutes
	if len(routes) != 3 {
		t.Fatalf("AdvertiseRoutes = %v, want 3 routes (subnet + all-v4 + all-v6)", routes)
	}
	found := false
	for _, r := range routes {
		if r == subnet {
			found = true
		}
	}
	if !found {
		t.Errorf("AdvertiseRoutes = %v, want to still contain %v", routes, subnet)
	}
}

func TestSetAdvertiseExitNodeTurnsOff(t *testing.T) {
	subnet := netip.MustParsePrefix("10.0.0.0/8")
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{
		AdvertiseRoutes: []netip.Prefix{subnet, tsaddr.AllIPv4(), tsaddr.AllIPv6()},
	}}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	routes := fake.gotMasked.Prefs.AdvertiseRoutes
	if len(routes) != 1 || routes[0] != subnet {
		t.Errorf("AdvertiseRoutes = %v, want only [%v]", routes, subnet)
	}
}

func TestSetAdvertiseExitNodeNoopWhenAlreadyDesiredState(t *testing.T) {
	fake := &fakeLocalClient{getPrefsResult: &ipn.Prefs{}}
	f := &Fetcher{lc: fake}

	got, err := f.SetAdvertiseExitNode(context.Background(), false)
	if err != nil {
		t.Fatalf("SetAdvertiseExitNode() error = %v, want nil", err)
	}
	if fake.editCallCount != 0 {
		t.Errorf("editCallCount = %d, want 0 (already false, should not call EditPrefs)", fake.editCallCount)
	}
	if got != fake.getPrefsResult {
		t.Errorf("SetAdvertiseExitNode() = %p, want the same *ipn.Prefs returned by GetPrefs (%p)", got, fake.getPrefsResult)
	}
}

func TestSetAdvertiseExitNodePropagatesGetPrefsError(t *testing.T) {
	fake := &fakeLocalClient{getPrefsErr: errors.New("get prefs failed")}
	f := &Fetcher{lc: fake}

	_, err := f.SetAdvertiseExitNode(context.Background(), true)
	if err == nil {
		t.Fatal("SetAdvertiseExitNode() error = nil, want an error")
	}
}
