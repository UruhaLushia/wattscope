package cli

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/pflag"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/internal/session"
)

func (o sourceOptions) recordingPath(output string, nextPath func(string) string) (string, error) {
	if output != "" {
		return output, nil
	}
	dir, err := recording.Directory(o.recordingsDir)
	if err != nil {
		return "", err
	}
	return nextPath(dir), nil
}

type acquisitionOptions struct {
	rate, history, packetHistory int
	timeout, duration            time.Duration
	reconnect                    bool
}

func (o *acquisitionOptions) flags(f *pflag.FlagSet, reconnect bool) {
	f.IntVar(&o.rate, "rate", 10, "")
	f.IntVar(&o.history, "history", 18000, "")
	f.IntVar(&o.packetHistory, "pd-history", 2000, "")
	f.DurationVar(&o.timeout, "timeout", 3*time.Second, "")
	f.DurationVar(&o.duration, "duration", 0, "")
	f.BoolVar(&o.reconnect, "reconnect", reconnect, "")
}

func service(source sourceOptions, o acquisitionOptions) (*backend.Service, error) {
	msgs := source.messages()
	if o.duration < 0 {
		return nil, errors.New(msgs.InvalidDuration())
	}
	if o.rate < 0 || o.rate > 1000 {
		return nil, errors.New(msgs.InvalidRate())
	}
	if o.history < 0 || o.history > 1000000 {
		return nil, errors.New(msgs.InvalidHistory())
	}
	if o.timeout != 0 && o.timeout < 100*time.Millisecond {
		return nil, errors.New(msgs.InvalidTimeout())
	}
	if o.packetHistory < 1 || o.packetHistory > 100000 {
		return nil, errors.New(msgs.InvalidPdHistory())
	}
	return session.New(session.Options{Source: source.source(), Rate: o.rate, HistorySize: o.history, PacketHistorySize: o.packetHistory, Reconnect: o.reconnect, StaleAfter: o.timeout})
}

func acquisitionContext(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	if duration > 0 {
		return context.WithTimeout(parent, duration)
	}
	return context.WithCancel(parent)
}
