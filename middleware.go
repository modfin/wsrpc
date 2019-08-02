package wsrpc

type Middleware func(c Context, next func() error) error

func processMiddleware(ctx Context, handler func() error, middleware ...Middleware) error {
	if middleware == nil || len(middleware) == 0 {
		return handler()
	}

	next := func() error {
		return processMiddleware(ctx, handler, middleware[1:]...)
	}

	return middleware[0](ctx, next)
}
