package uwebsocket

type sendOptions struct {
	messageFn func() ([]byte, error)
	filterFn  func(clientGUID string, attrs *ClientAttributes) bool
}

type SendOption func(*sendOptions)

// Specify the message to send
func WithMessage(message []byte) SendOption {
	return func(o *sendOptions) {
		o.messageFn = func() ([]byte, error) { return message, nil }
	}
}

// Specify the function to be called for generating
// the massage if the filter function returns true
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
