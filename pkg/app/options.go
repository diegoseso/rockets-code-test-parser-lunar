package app

import "go.uber.org/zap"

type options struct {
	LogOpts           []zap.Option
	DisableMonitoring bool
}

// Option defines functional options.
type Option func(*options)

// WithoutMonitoring disables OpenCensus exporters.
func WithoutMonitoring() Option {
	return func(opts *options) {
		opts.DisableMonitoring = true
	}
}

// WithZapOptions allows to pass custom logging options for Zap.
func WithZapOptions(zapopts ...zap.Option) Option {
	return func(opts *options) {
		opts.LogOpts = append(opts.LogOpts, zapopts...)
	}
}
