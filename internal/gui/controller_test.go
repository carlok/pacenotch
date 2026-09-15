package gui

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlok/pacenotch/internal/usage"
)

type recorder struct {
	mu    sync.Mutex
	views []View
	ch    chan View
}

func (r *recorder) publish(v View) {
	r.mu.Lock()
	r.views = append(r.views, v)
	r.mu.Unlock()
	if r.ch != nil {
		r.ch <- v
	}
}

func newController(t *testing.T, data []byte) (*Controller, *atomic.Int64, *atomic.Int32, *recorder) {
	var clock atomic.Int64
	clock.Store(testNow)
	var loads atomic.Int32
	rec := &recorder{}
	c := &Controller{
		Load: func(_ context.Context, force bool) (usage.Result, error) {
			loads.Add(1)
			return usage.Result{Data: data, Age: 0}, nil
		},
		Now:     func() time.Time { return time.Unix(clock.Load(), 0) },
		Band:    5,
		Publish: rec.publish,
	}
	return c, &clock, &loads, rec
}

func TestControllerRedrawMovesTheNotch(t *testing.T) {
	c, clock, loads, rec := newController(t, fixture(t, "api-sample"))
	if v := c.Redraw(); v.Header != Loading.Header {
		t.Fatalf("before the first load: %+v", v)
	}
	first := c.Refresh(context.Background(), true)
	clock.Add(3600)
	second := c.Redraw()
	if loads.Load() != 1 {
		t.Errorf("redraw must not load (%d loads)", loads.Load())
	}
	a, b := first.Rows[0].Row, second.Rows[0].Row
	if a.ExpBP >= b.ExpBP || a.Left <= b.Left {
		t.Errorf("an hour later the notch must move right: %d -> %d", a.ExpBP, b.ExpBP)
	}
	if len(rec.views) != 3 {
		t.Errorf("published %d views", len(rec.views))
	}
}

func TestControllerSetBand(t *testing.T) {
	c, _, loads, rec := newController(t, fixture(t, "api-sample"))
	c.Refresh(context.Background(), true)
	if rec.views[0].Rows[1].Row.Class != "ahead" {
		t.Fatalf("7d is +9 with band 5: %+v", rec.views[0].Rows[1].Row)
	}
	c.SetBand(10)
	if last := rec.views[len(rec.views)-1]; last.Rows[1].Row.Class != "on" || loads.Load() != 1 {
		t.Errorf("with band 10, +9 is on pace without reloading: %+v, loads %d", last.Rows[1].Row, loads.Load())
	}
}

func TestControllerRun(t *testing.T) {
	c, _, loads, rec := newController(t, fixture(t, "api-sample"))
	rec.ch = make(chan View, 8)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var every time.Duration
	go func() {
		c.Run(ctx, time.Minute, func(d time.Duration) <-chan time.Time { every = d; return ticks })
		close(done)
	}()
	<-rec.ch
	ticks <- time.Now()
	<-rec.ch
	cancel()
	<-done
	if loads.Load() != 2 || every != time.Minute {
		t.Errorf("loads %d, interval %v", loads.Load(), every)
	}
}

func TestControllerConcurrent(t *testing.T) {
	c, clock, _, _ := newController(t, fixture(t, "api-sample"))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				clock.Add(1)
				if j%3 == 0 {
					c.Refresh(context.Background(), false)
				} else if j%7 == 0 {
					c.SetBand(float64(j % 10))
				} else {
					c.Redraw()
				}
			}
		}()
	}
	wg.Wait()
}
