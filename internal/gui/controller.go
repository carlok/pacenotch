package gui

import (
	"context"
	"sync"
	"time"

	"github.com/carlok/pacenotch/internal/usage"
)

// Controller owns the data shared by the window and the tray: it loads, keeps the last
// result and publishes views. It is safe for concurrent use.
type Controller struct {
	Load    func(ctx context.Context, force bool) (usage.Result, error)
	Now     func() time.Time
	Band    float64
	Publish func(View) // called after every refresh or redraw, from the calling goroutine

	mu   sync.Mutex
	res  usage.Result
	err  error
	have bool
}

// Refresh loads (the loader fetches only when its TTL has expired, unless forced) and
// publishes the new view.
func (c *Controller) Refresh(ctx context.Context, force bool) View {
	res, err := c.Load(ctx, force)
	c.mu.Lock()
	c.res, c.err, c.have = res, err, true
	c.mu.Unlock()
	return c.Redraw()
}

// Redraw republishes the last data at the current time: the notch moves without new data.
func (c *Controller) Redraw() View {
	c.mu.Lock()
	v := Loading
	if c.have {
		v = BuildView(c.res, c.err, c.Now(), c.Band)
	}
	c.mu.Unlock()
	c.Publish(v)
	return v
}

// SetBand changes the "on pace" band and redraws.
func (c *Controller) SetBand(band float64) {
	c.mu.Lock()
	c.Band = band
	c.mu.Unlock()
	c.Redraw()
}

// Run refreshes now and then every interval until ctx ends.
func (c *Controller) Run(ctx context.Context, every time.Duration, after func(time.Duration) <-chan time.Time) {
	for {
		c.Refresh(ctx, false)
		select {
		case <-ctx.Done():
			return
		case <-after(every):
		}
	}
}
