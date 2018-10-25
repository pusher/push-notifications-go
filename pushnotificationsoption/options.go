package pushnotificationsoption

import (
	"time"
)

type Options struct {
	RequestTimeout *time.Duration
	BaseURLFormat  *string
}

type Option func(*Options)

func WithRequestTimeout(timeout time.Duration) Option {
	return func(args *Options) {
		args.RequestTimeout = &timeout
	}
}

func WithCustomBaseURL(urlFormat string) Option {
	return func(args *Options) {
		args.BaseURLFormat = &urlFormat
	}
}
