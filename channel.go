package wsrpc

import (
	"errors"
	"runtime"
	"sync"
)

var (
	ErrChanClosed = errors.New("channel closed")
)

type InfChannel struct {
	ch     chan interface{}
	once   sync.Once
	closed chan struct{}
}

func NewInfChannel() *InfChannel {
	return &InfChannel{
		ch:     make(chan interface{}),
		closed: make(chan struct{}),
	}
}

func (c *InfChannel) Close() {
	c.once.Do(func() {
		close(c.closed)
		close(c.ch)
	})
}

func (c *InfChannel) write(msg interface{}) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrChanClosed
		}
	}()

	select {
	case <-c.closed:
		return ErrChanClosed
	default:
	}

	select {
	case <-c.closed:
		return ErrChanClosed
	case c.ch <- msg:
	}

	return nil
}

func (c *InfChannel) read() (interface{}, error) {
	msg, ok := <-c.ch
	if !ok {
		return nil, ErrChanClosed
	}

	return msg, nil
}

func (c *InfChannel) clear() {
	c.Close()

	for {
		runtime.Gosched()

		_, err := c.read()
		if err != nil {
			break
		}
	}

}

type ResponseChannel struct {
	ch     chan *Response
	once   sync.Once
	mutex  sync.Mutex
	closed chan struct{}
}

func NewResponseChannel(size int) *ResponseChannel {
	return &ResponseChannel{
		ch:     make(chan *Response, size),
		closed: make(chan struct{}),
	}
}

func (c *ResponseChannel) Close() {
	c.once.Do(func() {
		c.mutex.Lock()
		close(c.closed)
		close(c.ch)
		c.mutex.Unlock()
	})
}

func (c *ResponseChannel) Closed() bool {
	select {
	case <-c.closed:
		return true
	default:
	}

	return false
}

func (c *ResponseChannel) Write(msg *Response) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrChanClosed
		}
	}()

	for !c.Closed() {
		select {
		case <-c.closed:
			return ErrChanClosed
		case c.ch <- msg:
			return nil
		default:
		}
	}

	return ErrChanClosed
}

func (c *ResponseChannel) read() (*Response, error) {
	msg, ok := <-c.ch
	if !ok {
		return nil, ErrChanClosed
	}

	return msg, nil
}
