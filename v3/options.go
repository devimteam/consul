package consul

import (
	"time"
)

type Logger interface {
	Log(...interface{}) error
}

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

func SetKV(kv KV) Option {
	return func(opts *options) {
		opts.kv = kv
	}
}

func Normalizer(f func(string) string) Option {
	return func(opts *options) {
		opts.normalizer = f
	}
}

func SetLogger(logger Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}
