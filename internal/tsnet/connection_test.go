package tsnet

import (
	"context"
	"errors"
	"testing"

	"tailscale.com/ipn"
)

func TestSetWantRunningTrue(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	got, err := f.SetWantRunning(context.Background(), true)
	if err != nil {
		t.Fatalf("SetWantRunning(true) error = %v, want nil", err)
	}
	if !fake.gotMasked.WantRunningSet || !fake.gotMasked.Prefs.WantRunning {
		t.Errorf("gotMasked = %+v, want WantRunningSet=true, WantRunning=true", fake.gotMasked)
	}
	if got == nil || !got.WantRunning {
		t.Errorf("SetWantRunning(true) = %+v, want WantRunning=true", got)
	}
}

func TestSetWantRunningFalse(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}

	_, err := f.SetWantRunning(context.Background(), false)
	if err != nil {
		t.Fatalf("SetWantRunning(false) error = %v, want nil", err)
	}
	if !fake.gotMasked.WantRunningSet || fake.gotMasked.Prefs.WantRunning {
		t.Errorf("gotMasked = %+v, want WantRunningSet=true, WantRunning=false", fake.gotMasked)
	}
}

func TestSetWantRunningPropagatesError(t *testing.T) {
	fake := &fakeLocalClient{editErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if _, err := f.SetWantRunning(context.Background(), true); err == nil {
		t.Fatal("SetWantRunning() error = nil, want non-nil")
	}
}

func TestListProfiles(t *testing.T) {
	current := ipn.LoginProfile{ID: "1ab3", Name: "alice@example.com"}
	all := []ipn.LoginProfile{current, {ID: "9f2c", Name: "bob@example.com"}}
	fake := &fakeLocalClient{profileStatusCurrent: current, profileStatusAll: all}
	f := &Fetcher{lc: fake}

	gotCurrent, gotAll, err := f.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles() error = %v, want nil", err)
	}
	if gotCurrent.ID != current.ID {
		t.Errorf("ListProfiles() current = %+v, want %+v", gotCurrent, current)
	}
	if len(gotAll) != 2 {
		t.Errorf("ListProfiles() all = %+v, want 2 profiles", gotAll)
	}
}

func TestListProfilesPropagatesError(t *testing.T) {
	fake := &fakeLocalClient{profileStatusErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if _, _, err := f.ListProfiles(context.Background()); err == nil {
		t.Fatal("ListProfiles() error = nil, want non-nil")
	}
}

func TestSwitchProfile(t *testing.T) {
	fake := &fakeLocalClient{}
	f := &Fetcher{lc: fake}
	id := ipn.ProfileID("9f2c")

	if err := f.SwitchProfile(context.Background(), id); err != nil {
		t.Fatalf("SwitchProfile() error = %v, want nil", err)
	}
	if fake.switchProfileGotID != id {
		t.Errorf("switchProfileGotID = %q, want %q", fake.switchProfileGotID, id)
	}
	if fake.switchProfileCallCount != 1 {
		t.Errorf("switchProfileCallCount = %d, want 1", fake.switchProfileCallCount)
	}
}

func TestSwitchProfilePropagatesError(t *testing.T) {
	fake := &fakeLocalClient{switchProfileErr: errors.New("boom")}
	f := &Fetcher{lc: fake}

	if err := f.SwitchProfile(context.Background(), "1ab3"); err == nil {
		t.Fatal("SwitchProfile() error = nil, want non-nil")
	}
}
