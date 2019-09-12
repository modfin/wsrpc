package wsrpc

// NextFunc is used to call the next middleware/handler when registering middlware
type NextFunc = func(ctx Context) error

// Middleware is passed to middlewares to allow chaining of multiple actions before and/or after calling the handler.
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
