package consul

import (
	"time"
)

type Option func(*options)

func OnlyPull(opts *options) {
	opts.onlyPull = true
}

func DisableWatch(opts *options) {
	opts.disableListen = true
}

func Period(period time.Duration) Option {
	return func(opts *options) {
		opts.refreshPeriod = period
	}
}
