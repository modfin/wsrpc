package wsrpc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
)

type Router struct {
	errc         chan error
	errPreProc   func(error) error
	errPostProc  func(error)
	wsUpgrade    websocket.Upgrader
	middleware   []Middleware
	rpcFunctions map[string]functionBundle
	rpcStreams   map[string]streamBundle
}

type SendFunction func(response *Response) error
type callHandler func(ctx Context) (err error)
type streamHandler func(ctx Context, ch *ResponseChannel) (err error)

type bundle struct {
	method     string
	middleware []Middleware
}

type functionBundle struct {
	bundle
	function callHandler
}

type streamBundle struct {
	bundle
	stream streamHandler
}

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

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// TODO: remove this case from production
	if req.RequestURI == "/client" {
		const basedir = "/go/src/github.com/modfin/wsrpc/resources/web/"

		b, err := ioutil.ReadFile(basedir + "index.tmpl.html")
		if err != nil {
			r.errc <- err
		}

		var buff []byte
		files, err := ioutil.ReadDir(basedir + "js/")
		if err != nil {
			r.errc <- err
		}

		for _, f := range files {
			buff = append(buff, '\n')

			bb, err := ioutil.ReadFile(basedir + "js/" + f.Name())
			if err != nil {
				r.errc <- err
			}

			buff = append(buff, bb...)
		}

		tmpl, err := template.New("test").Parse(string(b))
		if err != nil {
			r.errc <- err
		}

		err = tmpl.Execute(w, struct {
			JS string
		}{
			JS: string(buff),
		})
		if err != nil {
			r.errc <- err
		}

		return
	}
	// Case end

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

func (r *Router) Use(middleware ...Middleware) {
	r.middleware = append(r.middleware, middleware...)
}

func (r *Router) SetErrorPreProc(fn func(error) error) {
	r.errPreProc = fn
}

func (r *Router) SetErrorPostProc(fn func(error)) {
	r.errPostProc = fn
}

func (r *Router) SetHandler(method string, handler callHandler, middleware ...Middleware) {
	r.rpcFunctions[method] = functionBundle{
		bundle: bundle{
			method:     method,
			middleware: middleware,
		},
		function: handler,
	}
}

func (r *Router) SetStream(method string, handler streamHandler, middleware ...Middleware) {
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
			if websocket.IsCloseError(err, 1001) {
				return err
			}

			r.errc <- r.errPreProc(err)

			continue
		}

		if t != websocket.TextMessage {
			r.errc <- r.errPreProc(errUnsupportedFrame)
			continue
		}

		batch, err := createBatch(data)
		if err != nil {
			r.errc <- r.errPreProc(err)
		}

		sock.batches = append(sock.batches, batch)

		go r.routeRequest(batch, sock.channel)
	}
}

func (r *Router) startLongPoll(sock *socket) error {
	defer sock.channel.clear()

	data, err := ioutil.ReadAll(sock.req.Body)
	if err != nil {
		return err
	}

	batch, err := createBatch(data)
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

		handler, err := r.createHandler(job, batch.channel)
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
				if err == errChanClosed {
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

		exec := func() error {
			defer func() {
				if job.response.Result != nil || (job.response.Header != nil && len(job.response.Header) > 0) {
					err := jobc.Write(job.response)
					if err != nil {
						r.errc <- fmt.Errorf("exec handler could not response: %v", err)
					}
					return
				}

				rsp := job.NewResponse()
				rsp.Error = EOF()

				err := jobc.Write(rsp)
				if err != nil {
					r.errc <- fmt.Errorf("exec handler could not write error resp: %v", err)
				}
			}()

			err := rh.stream(job, jobc)
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

		exec := func() error {

			err := rh.function(job)
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
