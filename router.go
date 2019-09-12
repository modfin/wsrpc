package wsrpc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Router is used to demux incoming RPCs to the appropriate handlers.
type Router struct {
	errc         chan error
	errPreProc   func(error) error
	errPostProc  func(error)
	wsUpgrade    websocket.Upgrader
	middleware   []Middleware
	rpcFunctions map[string]functionBundle
	rpcStreams   map[string]streamBundle
}

// CallHandler is used to register a handler for RPCs which require exactly one response.
type CallHandler func(ctx Context) (err error)

// StreamHandler is sued to register handlers which need to be able to send an unknown number of responses for any given request.
type StreamHandler func(ctx Context, ch *ResponseChannel) (err error)

type bundle struct {
	method     string
	middleware []Middleware
}

type functionBundle struct {
	bundle
	function CallHandler
}

type streamBundle struct {
	bundle
	stream StreamHandler
}

// Returns a new router with default settings
func NewRouter() *Router {
	return &Router{
		wsUpgrade: websocket.Upgrader{
			HandshakeTimeout: 10 * time.Second,
		},
		errPreProc:   func(err error) error { return err },
		errPostProc:  func(err error) { fmt.Println("Router err: ", err) },
		errc:         make(chan error),
		rpcFunctions: make(map[string]functionBundle),
		rpcStreams:   make(map[string]streamBundle),
	}
}

// ServeHTTP is responsible for interpreting incoming HTTP requests and if appropriate upgrade the connection to web sockets.
// In either case it attempts to run the requested handler func.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	sock := newSocket(w, req)
	defer sock.kill()

	var err error
	switch req.Method {
	case http.MethodPost:
		err = r.startLongPoll(sock)
		if err != nil {
			r.errc <- err

			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case http.MethodGet:
		sock.conn, err = r.wsUpgrade.Upgrade(w, req, nil)
		if err != nil {
			r.errc <- err

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = r.startWS(sock)
		if err != nil {
			r.errc <- err

			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	default:
		r.errc <- errMethodNotFound
		http.Error(w, errMethodNotFound.Error(), http.StatusBadRequest)
	}

}

// Start is used to start a web server on the supplied address.
func (r *Router) Start(address string) error {
	defer close(r.errc)

	go func() {
		for {
			err, ok := <-r.errc
			if !ok {
				return
			}

			if r.errPostProc != nil {
				r.errPostProc(err)
			}
		}
	}()

	return http.ListenAndServe(address, r)
}

// Use applies middleware to router
func (r *Router) Use(middleware ...Middleware) {
	r.middleware = append(r.middleware, middleware...)
}

// SetErrorPreProc is called on an error before it is propagated back out.
func (r *Router) SetErrorPreProc(fn func(error) error) {
	r.errPreProc = fn
}

// SettErrorPostProc is called on errors that are propagated out of the system.
// Outside of middlewares this is the last chance to handle the error.
func (r *Router) SetErrorPostProc(fn func(error)) {
	r.errPostProc = fn
}

// SetHandler registers a call handler func.
func (r *Router) SetHandler(method string, handler CallHandler, middleware ...Middleware) {
	r.rpcFunctions[method] = functionBundle{
		bundle: bundle{
			method:     method,
			middleware: middleware,
		},
		function: handler,
	}
}

// SetStream registers a stream handler func.
func (r *Router) SetStream(method string, handler StreamHandler, middleware ...Middleware) {
	r.rpcStreams[method] = streamBundle{
		bundle: bundle{
			method:     method,
			middleware: middleware,
		},
		stream: handler,
	}
}

func (r *Router) startWS(sock *socket) error {
	go r.sendOutput(sock.conn, sock.channel)

	for {
		t, data, err := sock.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, 1001, 4000) {
				return err
			}

			r.errc <- r.errPreProc(err)

			continue
		}

		if t != websocket.TextMessage {
			r.errc <- r.errPreProc(errUnsupportedFrame)
			continue
		}

		batch, err := createBatch(data, sock.req)
		if err != nil {
			r.errc <- r.errPreProc(err)
		}

		sock.batches = append(sock.batches, batch)

		go r.routeRequest(batch, sock.channel)
	}
}

func (r *Router) startLongPoll(sock *socket) error {
	defer sock.kill()

	data, err := ioutil.ReadAll(sock.req.Body)
	if err != nil {
		return err
	}

	batch, err := createBatch(data, sock.req)
	if err != nil {
		r.errc <- r.errPreProc(err)
		return err
	}
	defer batch.kill()

	sock.batches = append(sock.batches, batch)

	go r.routeRequest(batch, sock.channel)

	msg, err := sock.channel.read()
	if err != nil {
		return err
	}
	batch.kill()

	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}

	sock.w.WriteHeader(http.StatusOK)
	_, err = sock.w.Write(data)
	if err != nil {
		r.errc <- r.errPreProc(err)
	}

	return nil
}

func (r *Router) routeRequest(batch *batch, outc *InfChannel) {
	defer batch.kill()

	batchc := batch.channel

	for i := range batch.jobs {
		job := batch.jobs[i]

		handler, err := r.createHandler(job, batchc)
		if err != nil {
			r.errc <- err

			resp := job.NewResponse()
			resp.Error = err

			err := batchc.Write(resp)
			if err != nil {
				r.errc <- err
			}

			continue
		}

		go func() {
			err := handler()
			if err != nil {
				resp := job.NewResponse()
				resp.Error = ServerError(err.Error())

				err = batchc.Write(resp)
				if err != nil {
					r.errc <- r.errPreProc(err)
				}
			}
		}()
	}

	if batch.isStream {
		runningJobs := len(batch.jobs)
		for runningJobs > 0 {
			res, err := batchc.read()
			if err != nil {
				if err == ErrChanClosed {
					runningJobs--
				}

				continue
			}

			if res.Error != nil && res.Error.Code == 205 {
				batch.killJob(res.Id)
				runningJobs--
			}
			err = outc.write(res)
			// Output channel is closed pleas cancel all..
			if err != nil {
				return
			}

		}
		return
	}

	result := make([]*Response, 0, len(batch.jobs))
	for range batch.jobs {
		res, err := batchc.read()
		if err != nil {
			continue
		}
		result = append(result, res)
	}

	var res interface{} = result
	if !batch.isSlice {
		res = result[0]
	}

	err := outc.write(res)
	if err != nil {
		r.errc <- r.errPreProc(err)
	}
}

func (r *Router) createHandler(job job, jobc *ResponseChannel) (func() error, *Error) {
	var handler func() error

	switch job.request.Type {
	case TypeStream:
		rh, exists := r.rpcStreams[job.request.Method]
		if !exists {
			return nil, MethodNotFoundError(job.request.Method)
		}

		exec := func(cc Context) error {
			defer func() {
				select {
				case <-cc.Done():
					return
				default:
				}

				if cc.Response().Result != nil || (cc.Response().Header != nil && len(cc.Response().Header) > 0) {
					err := jobc.Write(cc.Response())
					if err != nil {
						r.errc <- fmt.Errorf("exec handler could not send respose: %v", err)
					}
				}

				rsp := cc.NewResponse()
				rsp.Error = EOF()

				err := jobc.Write(rsp)
				if err != nil {
					r.errc <- fmt.Errorf("exec handler could not write error resp: %v", err)
				}
			}()

			err := rh.stream(cc, jobc)
			if err != nil {
				return err
			}

			return nil
		}

		handler = func() error {
			return processMiddleware(job, exec, append(r.middleware, rh.middleware...)...)
		}

	case TypeCall:
		rh, exists := r.rpcFunctions[job.request.Method]
		if !exists {
			return nil, MethodNotFoundError(job.request.Method)
		}

		exec := func(cc Context) error {

			err := rh.function(cc)
			if err != nil {
				return err
			}

			jobc.ch <- job.response

			return nil
		}

		handler = func() error {
			return processMiddleware(job, exec, append(r.middleware, rh.middleware...)...)
		}

	default: // Type not found
		return nil, TypeNotFoundError(job.request.Type)
	}

	return handler, nil
}

func (r *Router) sendOutput(conn *websocket.Conn, outputc *InfChannel) {
	for {
		res, err := outputc.read()
		if err != nil {
			return
		}

		data, err := json.Marshal(res)
		if err != nil {
			r.errc <- err

			continue
		}

		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			r.errc <- err
		}
	}
}
