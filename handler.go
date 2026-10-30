package uwebsocket

import "github.com/dunv/uhttp"

func NewHandler(opts ...HandlerOption) Handler {
	mergedOpts := &handlerOptions{
		uhttpHandler:         uhttp.NewHandler(),
		incomingMessageLimit: 64,
	}
	for _, opt := range opts {
		opt.apply(mergedOpts)
	}
	validateSnapshotHandler(*mergedOpts)
	return Handler{wsOpts: *mergedOpts}
}

type Handler struct {
	wsOpts handlerOptions
}
