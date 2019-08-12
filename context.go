package wsrpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	ErrContextCancelled = errors.New("context cancelled")
)

type Context interface {
	context.Context

	Request() *Request
	Response() *Response
	NewResponse() *Response
	WithValue(key interface{}, value interface{}) Context
}

type socket struct {
	ctx    context.Context
	cancel func()
	once   sync.Once

	conn *websocket.Conn
	w    http.ResponseWriter
	req  *http.Request

	channel *InfChannel
	batches []*batch
}

func newSocket(w http.ResponseWriter, req *http.Request) *socket {
	ctx, cancel := context.WithCancel(req.Context())
	return &socket{
		ctx:     ctx,
		cancel:  cancel,
		w:       w,
		req:     req,
		channel: NewInfChannel(),
		batches: make([]*batch, 0),
	}
}

func (s *socket) kill() {
	s.once.Do(func() {
		for i := range s.batches {
			s.batches[i].kill()
		}
		s.channel.clear()
		s.cancel()
	})

}

type batch struct {
	ctx    context.Context
	cancel func()
	once   sync.Once

	isSlice  bool `json:"-"`
	isStream bool `json:"-"`

	channel *ResponseChannel
	jobs    []job
}

func createBatch(data []byte) (*batch, error) {
	ctx, cancel := context.WithCancel(context.Background())
	batch := batch{
		ctx:    ctx,
		cancel: cancel,
	}

	if len(data) > 0 && data[0] == '[' {
		batch.isSlice = true
		var requests []Request
		err := json.Unmarshal(data, &requests)
		if err != nil {
			return nil, err
		}

		for i := range requests {
			req := requests[i]

			ctx, cancel := context.WithCancel(context.Background())

			batch.jobs = append(batch.jobs, job{
				Context:  ctx,
				cancel:   cancel,
				request:  &req,
				response: newResponse(req.Id, nil),
			})
		}

	}

	if len(data) > 0 && data[0] == '{' {
		batch.isSlice = false

		req := newRequest()
		err := json.Unmarshal(data, &req)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithCancel(context.Background())
		batch.jobs = []job{
			{
				Context:  ctx,
				cancel:   cancel,
				request:  &req,
				response: newResponse(req.Id, nil),
			},
		}
	}

	if len(batch.jobs) < 1 {
		return nil, errMissingRequest
	}

	batch.isStream = batch.jobs[0].request.Type == TypeStream
	for _, job := range batch.jobs {
		if job.request.Id == 0 {
			return nil, errMissingRequestId
		}

		if (job.request.Type == TypeStream) != batch.isStream {
			return nil, errMixedTypes
		}
	}

	batch.channel = NewResponseChannel(len(batch.jobs))

	return &batch, nil
}

func (b *batch) kill() {
	b.once.Do(func() {
		for i := range b.jobs {
			b.jobs[i].kill()
		}
		b.channel.Close()
		b.cancel()
	})
}

func (b *batch) killJob(id int) {
	for i := range b.jobs {
		if b.jobs[i].request.Id == id {
			b.jobs[i].kill()
		}
	}
}

type job struct {
	context.Context

	cancel   func()
	once     sync.Once
	request  *Request
	response *Response
}

func (j *job) kill() {
	j.once.Do(j.cancel)
}

func (j job) NewResponse() *Response {
	return newResponse(j.Request().Id, nil)
}

func (j job) Request() *Request {
	return j.request
}

func (j job) Response() *Response {
	return j.response
}

func (j job) WithValue(key interface{}, value interface{}) Context {
	j.Context = context.WithValue(j.Context, key, value)

	return j
}
