package wsrpc

type NextFunc = func(ctx Context) error

type Middleware func(c Context, next NextFunc) error

func processMiddleware(ctx Context, handler NextFunc, middleware ...Middleware) error {
	if middleware == nil || len(middleware) == 0 {
		return handler(ctx)
	}

	next := func(c Context) error {
		return processMiddleware(c, handler, middleware[1:]...)
	}

	return middleware[0](ctx, next)
}
