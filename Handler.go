package uwebsocket

import "github.com/dunv/uhttp"

func NewHandler(opts ...HandlerOption) Handler {
	mergedOpts := &handlerOptions{
		uhttpHandler: uhttp.NewHandler(),
	}
	for _, opt := range opts {
		opt.apply(mergedOpts)
	}
	return Handler{wsOpts: *mergedOpts}
}

type Handler struct {
	wsOpts handlerOptions
}
