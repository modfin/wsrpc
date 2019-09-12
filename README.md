# Web Socket RPC
A simple web socket api framework for sending json rpc requests.
The framework is similar too but not necessarily compliant with the json rpc 2.0 standard.

[![GoDoc](https://godoc.org/github.com/modfin/wsrpc?status.svg)](https://godoc.org/github.com/modfin/wsrpc)
[![Go Report Card](https://goreportcard.com/badge/github.com/modfin/wsrpc)](https://goreportcard.com/report/github.com/modfin/wsrpc)
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/modfin/wsrpc/master/LICENSE.md)

Web sockets are the primary transport method but the framework allows the client to fallback on long polls in case of unsuitable conditions.

A client library is available at: http://github.com/modfin/wsrpc-js

## Implementing the server
The framework is similar to regular http frameworks by design to make it more intuitive, you have access to common resources like:
* Middlewares 
* Context
* Registering Handlers
* Sticky Headers
* Cookies
* The raw HTTP request

### Create the router
```go
    import "github.com/modfin/wsrpc"

    ...

    router := wsrpc.NewRouter()
```
### Apply middleware you wish to use or skip.
Middlewares can be appended to the router to affect all handlers on the router. It can also be specified in a specific handler registration to affect only that handler.
```go
func someMiddleware(c wsrpc.Context, next wsrpc.NextFunc) error {
    err := next(ctx)
    if err != nil {
        log.Error(err)
    }

    return nil
}

    ...


func main() {
    ...
    router.Use(someMiddleware)
    ...
}
```

### Extracting parameters
Paramaters are passed to the handler contexts request object as a raw json message.
```go
func someCallHandler(ctx wsrpc.Context) error {
    ...
    var num int64
    err := json.Unmarshal(ctx.Request().Params, &num)
    if err != nil {
        return err
    }

    ...

}
```

### Extracting and setting headers
Headers are passed to the handler contexts request object as key value objects.
```go
func someCallHandler(ctx wsrpc.Context) error {
    ...
	num, ok := ctx.Request().Header.Get("state").Int()
	if !ok {
		return errors.New("missing valid request state")
	}
    ...

}
```

### Working with the Context
The wsrpc Context used by handlers implements the standard go Context interface in addition to it's own features.

Context cancellations will cascade down from the top level connection down to each the connections batch of jobs and finally down to each job in said batches. Allowing us to cancel any action based on the handlers context. 
```go
func someStreamHandler(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) error {
	countdown := 3
	
	for countdown > 0 {
		select {
			case <-ctx.Done():
				log.Println("context cancelled")
				return nil
			default:
		}
		
		countdown -= 1
	}
	
	return nil
}
``` 

### The raw HTTP request
If we need something from the original HTTP request, e.g. the original request URL we can access it through the handler context
```go
func someCallHandler(ctx wsrpc.Context) error {
...
	connectionUrl := ctx.HttpRequest().URL.String()
...
}
```

### Sending responses to the client
Responses can be sent by setting the Content field on the contexts response object. THe response is automatically sent to the client when the handler returns. 

If you wish to trigger the response right away, i.e. in the case of a streaming handler with multiple responses there is a channel available to stream handlers.
```go
func someCallHandler(ctx wsrpc.Context) error {
...
	ctx.Response().Result, err = json.Marshal(num * num)
	if err != nil {
		return nil
	}}
...
}

func someStreamHandler(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) error {
...
		rsp := ctx.NewResponse()
		
		var err error 
		rsp.Result, err = json.Marshal(42)
		if err != nil {
			return err
		}
		
		err = ch.Write(rsp)
		if err != nil {
			return err
		}
...
}
```

### Registering Handlers
There are two handler funcs available. One for simple call and return once jobs, and one for streaming jobs where the server may respond an unknown number of times.
Keep in min that a handler is expected to function in both web socket and long poll mode. Make sure your handlers are reentrant for long polls.
```go
// type CallHandler func(ctx Context) (err error)
// type StreamHandler func(ctx Context, ch *ResponseChannel) (err error)

func main() {
    ...
    router.SetHandler("answerLife", someCallHandler)

    router.SetStream("countdown", someStreamHandler)
    ...

}

func myCallHandler(ctx wsrpc.Context) error {
    var err error
    ctx.Response().Content, err = json.Marshal(42)
    if err != nil {
        return err
    }
    
    return nil
}

func myStreamHandler(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) (err error) {
    countdown := 3

    for countdown > 0 {
        rsp := ctx.NewResponse()
        rsp.Content, err = json.Marshal(countdown)
        if err != nil {
           return err
        }
     
        err = ch.Write(rsp)
        if err != nil {
            return err
        }

        countdown -= 1
    }

    return nil
}
```

### A small reference setup
```go
package main

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"

	"github.com/modfin/wsrpc"
)

func main() {
	router := wsrpc.NewRouter()

    router.Use(someMiddleware)
	
	router.SetHandler("cubify", someCallHandler, func(ctx wsrpc.Context, next wsrpc.NextFunc) error {
		if rand.Int() < 53200 {
			return errors.New("user is not authenticated")
		}
		
		return next(ctx)
	})
	
	router.SetStream("countdown", someStreamHandler)
	
	err := router.Start(":8080")
	if err != nil {
		log.Fatal(err)
	}
}

func someMiddleware(ctx wsrpc.Context, next wsrpc.NextFunc) error {
    err := next(ctx)
    if err != nil {
    	log.Println(err)
        return err
    }

    return next(ctx)
}

func someCallHandler(ctx wsrpc.Context) error {
	var num int64
	err := json.Unmarshal(ctx.Request().Params, &num)
	if err != nil {
		return err
	}

	ctx.Response().Result, err = json.Marshal(num * num)
	if err != nil {
		return nil
	}

	return nil
}

func someStreamHandler(ctx wsrpc.Context, ch *wsrpc.ResponseChannel) error {
	countdown, ok := ctx.Request().Header.Get("countdown").Int()
	if !ok {
		return errors.New("missing countdown")
	}

	for countdown > 0 {
		select {
			case <-ctx.Done():
				log.Println("context cancelled")
				return nil
			default:
		}

		rsp := ctx.NewResponse()

		var err error
		rsp.Result, err = json.Marshal(countdown)
		if err != nil {
			return err
		}

		err = ch.Write(rsp)
		if err != nil {
			return err
		}

		countdown -= 1
	}

	return nil
}
```

## General guidelines
### Server
* Registered handlers are responsible for checking the provided input
* Stream handlers channels go straight to client, mind your output

## Issues
* The error handling could probably be done with middleware, alternatively a logger could be attached
* Attaching request IDs to each context will make debugging and error tracing easier 
* A data race occurs for both wrapped channel types when a stream handler is called with long polling.