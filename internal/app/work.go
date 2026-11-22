package app

import (
	"context"

	"github.com/egoist/mygo"
)

func (d *dashboard) dispatchTo(window *mygo.Window) func(func()) {
	ctx := d.ctx
	return func(apply func()) {
		if ctx.Err() != nil {
			return
		}
		finish := func() {
			if ctx.Err() == nil && (window == nil || !window.IsDestroyed()) {
				apply()
			}
		}
		if window != nil {
			window.Update(finish)
			return
		}
		d.uiMu.Lock()
		d.uiQueue = append(d.uiQueue, finish)
		d.uiMu.Unlock()
	}
}

func (d *dashboard) drainUIQueue() {
	d.uiMu.Lock()
	queued := d.uiQueue
	d.uiQueue = nil
	d.uiMu.Unlock()
	for _, apply := range queued {
		apply()
	}
}

func (d *dashboard) runJob(run func(context.Context) (func(), string, error)) {
	if d.busy || d.ctx.Err() != nil {
		return
	}
	d.busy = true
	ctx := d.ctx
	dispatch := d.dispatchTo(d.window)
	d.work.Go(func() {
		if ctx.Err() != nil {
			return
		}
		apply, message, err := run(ctx)
		dispatch(func() {
			d.busy = false
			if err != nil {
				d.notify(err.Error(), "error")
				return
			}
			if apply != nil {
				apply()
			}
			if message != "" {
				d.notify(message, "success")
			}
		})
	})
}

func awaitDialog[Value any](ctx context.Context, show func() (Value, error)) (Value, error) {
	type result struct {
		value Value
		err   error
	}
	var zero Value
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	done := make(chan result, 1)
	go func() {
		if err := ctx.Err(); err != nil {
			done <- result{err: err}
			return
		}
		value, err := show()
		done <- result{value, err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case completed := <-done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return completed.value, completed.err
	}
}
