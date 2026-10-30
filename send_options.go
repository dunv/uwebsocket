package uwebsocket

type sendOptions struct {
	messageFn func() ([]byte, error)
	filterFn  func(clientGUID string, attrs *ClientAttributes) bool
}

type SendOption func(*sendOptions)

// WithMessage supplies immutable bytes shared by all matching clients. Do not
// modify the slice after sending; network writes may still be pending.
func WithMessage(message []byte) SendOption {
	return func(o *sendOptions) {
		o.messageFn = func() ([]byte, error) { return message, nil }
	}
}

// WithMessageFn builds one immutable payload when at least one client matches.
// The callback runs synchronously and is serialized against other Send calls.
func WithMessageFn(fn func() ([]byte, error)) SendOption {
	return func(o *sendOptions) {
		o.messageFn = fn
	}
}

// Specify the filter function to be called for each client
func WithFilterFn(fn func(clientGUID string, attrs *ClientAttributes) bool) SendOption {
	return func(o *sendOptions) {
		o.filterFn = fn
	}
}

func WithFlagFilter(flag string) SendOption {
	return func(o *sendOptions) {
		o.filterFn = func(clientGUID string, attrs *ClientAttributes) bool {
			return attrs.IsFlagSet(flag)
		}
	}
}

func WithMatchFilter(key string, value string) SendOption {
	return func(o *sendOptions) {
		o.filterFn = func(clientGUID string, attrs *ClientAttributes) bool {
			return attrs.HasMatch(key, value)
		}
	}
}

func WithClientFilter(clientGUID string) SendOption {
	return func(o *sendOptions) {
		o.filterFn = func(guid string, attrs *ClientAttributes) bool {
			return guid == clientGUID
		}
	}
}
