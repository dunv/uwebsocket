package uwebsocket

type sendOptions struct {
	messageFn func() ([]byte, error)
	async     bool
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

// Send the message asynchronously
func WithAsync() SendOption {
	return func(o *sendOptions) {
		o.async = true
	}
}
