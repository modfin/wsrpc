package wsrpc

import (
	"errors"
	"runtime"
	"sync"
)

var (
	// ErrChanClosed is returned when writing to or reading from a closed channel
	ErrChanClosed = errors.New("channel closed")
)

// InfChannel is used to pass data of unspecified types inside the wsrpc server.
// It is not meant to be made available inside a request handler like it's sibling ResponseChannel is
type InfChannel struct {
	ch     chan interface{}
	once   sync.Once
	closed chan struct{}
}

// NewInfChannel returns a new InfChannel
func NewInfChannel() *InfChannel {
	return &InfChannel{
		ch:     make(chan interface{}),
		closed: make(chan struct{}),
	}
}

// Close closes the channel.
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

// ResponseChannel is passed to stream handlers to allow the implementer an easy way of sending responses back to the requester.
type ResponseChannel struct {
	ch     chan *Response
	once   sync.Once
	mutex  sync.Mutex
	closed chan struct{}
}

// NewResponseChannel returns a new ResponseChannel.
func NewResponseChannel(size int) *ResponseChannel {
	return &ResponseChannel{
		ch:     make(chan *Response, size),
		closed: make(chan struct{}),
	}
}

// Close closes the ResponseChannel.
func (c *ResponseChannel) Close() {
	c.once.Do(func() {
		c.mutex.Lock()
		close(c.closed)
		close(c.ch)
		c.mutex.Unlock()
	})
}

// Closed checks wether or not a ResponseChannel is closed.
func (c *ResponseChannel) Closed() bool {
	select {
	case <-c.closed:
		return true
	default:
	}

	return false
}

// Write sends a message through the ResponseChannel.
// Stream handlers are configured to pass this message back to the requester if possible.
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
