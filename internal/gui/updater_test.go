package gui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/carlok/pacenotch/internal/update"
)

func TestUpdater(t *testing.T) {
	st := mapStore{}
	clock := time.Unix(1789126200, 0)
	checks := 0
	answer, answerErr := update.Release{Tag: "v0.4.0", URL: update.ReleasesPrefix + "tag/v0.4.0"}, error(nil)
	var found []update.Release
	enabled := false
	u := &Updater{
		Check:   func(context.Context) (update.Release, error) { checks++; return answer, answerErr },
		Store:   st,
		Now:     func() time.Time { return clock },
		Version: "v0.3.0",
		Enabled: func() bool { return enabled },
		OnFound: func(r update.Release) { found = append(found, r) },
	}
	ctx := context.Background()

	u.Tick(ctx)
	if checks != 0 || len(found) != 0 {
		t.Fatal("disabled: no check")
	}
	enabled = true
	u.Tick(ctx)
	if checks != 1 || len(found) != 1 || found[0] != answer || st[PrefUpdateLastCheck] != int(clock.Unix()) {
		t.Fatalf("first check: checks %d found %v store %v", checks, found, st)
	}
	clock = clock.Add(time.Hour)
	u.Tick(ctx)
	if checks != 1 || len(found) != 2 {
		t.Fatalf("not due: the known release is still reported, without asking GitHub: checks %d found %d", checks, len(found))
	}

	clock = clock.Add(update.Interval)
	answerErr = update.ErrRateLimited
	u.Tick(ctx)
	if checks != 2 || st[PrefUpdateLastCheck] != int(clock.Unix()) || len(found) != 3 {
		t.Fatalf("a failed check waits a day and keeps the known release: checks %d store %v", checks, st)
	}

	u.Version = "v0.4.0"
	u.Tick(ctx)
	if len(found) != 3 {
		t.Error("no notice when up to date")
	}
	u.Version = "v0.1.3-5-g84f4fdf"
	u.Tick(ctx)
	if len(found) != 3 {
		t.Error("no notice for local builds")
	}
	u.Version = "v0.3.0"
	st[PrefUpdateURL] = "https://example.com/evil"
	u.Tick(ctx)
	if len(found) != 3 {
		t.Error("a stored address outside the releases page is ignored")
	}
}

func TestUpdaterRun(t *testing.T) {
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	checks := make(chan struct{}, 4)
	clock := time.Unix(1789126200, 0)
	u := &Updater{
		Check: func(context.Context) (update.Release, error) {
			checks <- struct{}{}
			return update.Release{}, errors.New("offline")
		},
		Store: mapStore{}, Version: "v0.1.0", Enabled: func() bool { return true },
		Now: func() time.Time { clock = clock.Add(25 * time.Hour); return clock }, OnFound: func(update.Release) {},
	}
	done := make(chan struct{})
	var every time.Duration
	go func() {
		u.Run(ctx, time.Hour, func(d time.Duration) <-chan time.Time { every = d; return ticks })
		close(done)
	}()
	<-checks
	ticks <- time.Now()
	<-checks
	cancel()
	<-done
	if every != time.Hour {
		t.Errorf("interval %v", every)
	}
}

func TestUpdateText(t *testing.T) {
	if UpdateText(update.Release{}) != "" || UpdateText(update.Release{Tag: "v1.0.0"}) != "Update available: v1.0.0" {
		t.Error("UpdateText")
	}
}
