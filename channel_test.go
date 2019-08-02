package wsrpc

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

const defaultString = "Hello wonderful world of unit testing"

func TestNewInfChannel(t *testing.T) {
	ch := NewInfChannel()

	tt := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		{
			name:     "underlying chan",
			value:    ch.ch,
			expected: make(chan interface{}),
		},
		{
			name:     "close chan",
			value:    ch.closed,
			expected: make(chan struct{}),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == nil {
				t.Fatalf("expected %+v; got %+v", tc.value, nil)
			}

			if reflect.TypeOf(tc.value) != reflect.TypeOf(tc.expected) {
				t.Fatalf("expected type %+v; got %+v", reflect.TypeOf(tc.value), reflect.TypeOf(tc.expected))
			}
		})
	}
}

func TestInfChannel_Close(t *testing.T) {
	tt := []struct {
		name             string
		preparation      func(*InfChannel) *InfChannel
		expectChOpen     bool
		expectClosedOpen bool
	}{
		{
			name: "single close",
			preparation: func(ch *InfChannel) *InfChannel {
				return ch
			},
			expectChOpen:     false,
			expectClosedOpen: false,
		},
		{
			name: "double close",
			preparation: func(ch *InfChannel) *InfChannel {
				ch.Close()
				return ch
			},
			expectChOpen:     false,
			expectClosedOpen: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewInfChannel()
			ch = tc.preparation(ch)

			defer func() {
				r := recover()
				if r != nil {
					t.Fatalf("received panic: %+v", r)
				}
			}()

			ch.Close()

			_, open := <-ch.ch
			if open != tc.expectChOpen {
				t.Errorf("underlying chan %v; expected %v", open, tc.expectChOpen)
			}

			_, open = <-ch.closed
			if open != tc.expectClosedOpen {
				t.Errorf("close chan %v; expected %v", open, tc.expectClosedOpen)
			}
		})
	}
}

func TestInfChannel_write(t *testing.T) {
	tt := []struct {
		name          string
		msg           interface{}
		withReader    bool
		expectedErr   error
		expectTimeout bool
		preparation   func(channel *InfChannel)
	}{
		{name: "Simple write string", msg: defaultString, withReader: true, expectedErr: nil, expectTimeout: false},
		{name: "Simple write int", msg: 12, withReader: true, expectedErr: nil, expectTimeout: false},
		{name: "Write without reader", msg: defaultString, withReader: false, expectedErr: nil, expectTimeout: true},
		{name: "Write on closed chan", msg: defaultString, withReader: false, expectedErr: errChanClosed, expectTimeout: false,
			preparation: func(ch *InfChannel) {
				ch.Close()
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewInfChannel()
			defer ch.Close()

			if tc.withReader {
				go func() {
					_, err := ch.read()
					if err != nil && err != tc.expectedErr {
						t.Fatalf("unexpected error while reading: %v", err)
					}
				}()
			}

			if tc.preparation != nil {
				tc.preparation(ch)
			}

			retc := make(chan error)

			started := make(chan struct{})
			go func() {
				started <- struct{}{}
				select {
				case <-time.After(50 * time.Millisecond):
				case retc <- ch.write(tc.msg):
				}
				close(retc)

			}()

			<-started

			select {
			case <-time.After(100 * time.Millisecond):
				if !tc.expectTimeout {
					t.Fatal("unexpected timeout while writing")
				}
			case err := <-retc:
				if err != tc.expectedErr {
					t.Fatalf("expected %v; got %v", tc.expectedErr, err)
				}

				if tc.expectTimeout {
					t.Fatalf("expected timeout; got %v", err)
				}
			}
		})
	}
}

func TestInfChannel_read(t *testing.T) {
	type readRet struct {
		msg interface{}
		err error
	}
	tt := []struct {
		name          string
		expectedMsg   interface{}
		expectedErr   error
		expectTimeout bool
		withWriter    bool
		preparation   func(ch *InfChannel)
	}{
		{name: "Simple read string", expectedMsg: defaultString, withWriter: true},
		{name: "Simple read int", expectedMsg: 12, withWriter: true},
		{name: "Read without writer", expectedMsg: defaultString, expectTimeout: true},
		{name: "Read closed chan", expectedErr: errChanClosed,
			preparation: func(ch *InfChannel) {
				ch.Close()
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewInfChannel()

			var msg interface{} = defaultString
			if tc.expectedMsg != nil {
				msg = tc.expectedMsg
			}

			if tc.withWriter {
				go func() {
					_ = ch.write(msg)
				}()
				time.Sleep(1 * time.Second)
			}

			if tc.preparation != nil {
				tc.preparation(ch)
			}

			retc := make(chan readRet)
			defer close(retc)

			go func() {
				intRetc := make(chan readRet)
				defer close(intRetc)

				go func() {
					msg, err := ch.read()
					if err == errChanClosed {
						fmt.Println(tc.name, msg, err)
					}
					intRetc <- readRet{msg, err}
				}()

				select {
				case <-time.After(1 * time.Second):
					return
				case retc <- <-intRetc:
				}
			}()

			select {
			case <-time.After(2 * time.Second):
				if !tc.expectTimeout {
					t.Fatal("unexpected timeout")
				}
			case readRet := <-retc:
				msg := readRet.msg
				err := readRet.err

				if err != tc.expectedErr {
					t.Fatalf("expected err to be %v; got %v", tc.expectedErr, err)
				}

				if msg != tc.expectedMsg {
					t.Fatalf("expected %v; got %v", tc.expectedMsg, msg)
				}

				if tc.expectTimeout {
					t.Fatalf("expected timeout; got %v", err)
				}
			}

		})
	}
}

func TestInfChannel_clear(t *testing.T) {
	tt := []struct {
		name        string
		withReader  bool
		withWriter  bool
		expectedErr error
	}{
		{name: "empty chan"},
		{name: "with reader", withReader: true, expectedErr: errChanClosed},
		{name: "with writer", withWriter: true, expectedErr: errChanClosed},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewInfChannel()

			stopReader := make(chan struct{})
			if tc.withReader {
				go func() {
					for {
						select {
						case <-stopReader:
							return
						default:
							_, err := ch.read()
							if err != nil && err != tc.expectedErr {
								t.Fatalf("unexpected err while reading: %v", err)
							}
						}
					}
				}()
			}

			stopWriter := make(chan struct{})
			if tc.withWriter {
				go func() {
					defer ch.Close()

					for {
						select {
						case <-stopWriter:
							return
						default:
							err := ch.write(defaultString)
							if err != nil && err != tc.expectedErr {
								t.Fatalf("unexpected err while reading: %v", err)
							}
						}
					}
				}()
			}

			ch.clear()
			close(stopReader)
			close(stopWriter)

			select {
			case <-time.After(100 * time.Millisecond):
				t.Fatal("unexpected timeout while checking closed")
			case <-ch.closed:
			}
		})
	}
}

func TestNewResponseChannel(t *testing.T) {
	tt := []struct {
		name string
		size int
	}{
		{name: "no buffer", size: 0},
		{name: "with buffer", size: 10},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewResponseChannel(tc.size)

			if ch.ch == nil || ch.closed == nil {
				t.Fatalf("expected underlying chans to be open, found: ch %v, closed: %v", ch.ch, ch.closed)
			}

			if cap(ch.ch) != tc.size {
				t.Fatalf("expected underlying chan to have a buffer of size %v; got %v", tc.size, cap(ch.ch))
			}

			if ch.Closed() {
				t.Fatal("new chan unexpectedly closed")
			}
		})
	}
}

func TestResponseChannel_Close(t *testing.T) {
	tt := []struct {
		name             string
		size             int
		expectMsg        bool
		expectChOpen     bool
		expectClosedOpen bool
		preparation      func(*ResponseChannel)
	}{
		{name: "single close"},
		{name: "double close",
			preparation: func(ch *ResponseChannel) {
				ch.Close()
			},
		},
		{name: "close buffered chan", size: 2, expectChOpen: true, expectMsg: true,
			preparation: func(ch *ResponseChannel) {
				_ = ch.Write(&Response{Id: 1})
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewResponseChannel(tc.size)

			if tc.preparation != nil {
				tc.preparation(ch)
			}

			defer func() {
				r := recover()
				if r != nil {
					t.Fatalf("received panic: %+v", r)
				}
			}()

			ch.Close()

			msg, open := <-ch.ch
			if open != tc.expectChOpen {
				t.Errorf("underlying chan %v; expected %v", open, tc.expectChOpen)
			}
			if msg != nil && !tc.expectMsg {
				t.Fatalf("expected blank msg; got %v", msg)
			}

			_, open = <-ch.closed
			if open != tc.expectClosedOpen {
				t.Errorf("close chan %v; expected %v", open, tc.expectClosedOpen)
			}
		})
	}
}

func TestResponseChannel_write(t *testing.T) {
	tt := []struct {
		name          string
		size          int
		withReader    bool
		expectedErr   error
		expectTimeout bool
		preparation   func(ch *ResponseChannel) error
	}{
		{name: "Simple write", withReader: true},
		{name: "Write without reader", expectTimeout: true},
		{name: "Write on full chan", size: 1, expectTimeout: true,
			preparation: func(ch *ResponseChannel) error {
				return ch.Write(&Response{})
			},
		},
		{name: "Write on closed chan", expectedErr: errChanClosed,
			preparation: func(ch *ResponseChannel) error {
				ch.Close()

				return nil
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewResponseChannel(tc.size)
			defer ch.Close()

			if tc.withReader {
				go func() {
					_, _ = ch.read()
				}()
			}

			if tc.preparation != nil {
				err := tc.preparation(ch)
				if err != nil {
					t.Fatalf("received err during preparation: %v", err)
				}
			}

			retc := make(chan error)
			go func() {
				defer close(retc)

				select {
				case <-time.After(1 * time.Second):
					return
				case retc <- ch.Write(&Response{}):
				}
			}()

			select {
			case <-time.After(2 * time.Second):
				if !tc.expectTimeout {
					t.Fatal("unexpected timeout while writing")
				}
			case err := <-retc:
				if err != tc.expectedErr {
					t.Fatalf("expected %v; got %v", tc.expectedErr, err)
				}

				if tc.expectTimeout {
					t.Fatalf("expected timeout; got %v", err)
				}
			}
		})
	}
}

func TestResponseChannel_read(t *testing.T) {
	tt := []struct {
		name          string
		size          int
		expectedErr   error
		expectTimeout bool
		withWriter    bool
		preparation   func(ch *ResponseChannel)
	}{
		{name: "Simple read", withWriter: true},
		{name: "Read without writer", expectTimeout: true},
		{name: "Read empty chan", size: 1, expectTimeout: true},
		{name: "Read closed chan", expectedErr: errChanClosed,
			preparation: func(ch *ResponseChannel) {
				ch.Close()
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewResponseChannel(tc.size)

			if tc.withWriter {
				go func() {
					err := ch.Write(&Response{})
					if err != nil {
						t.Fatalf("unexpected err while writing: %v", err)
					}
				}()
				time.Sleep(1 * time.Second)
			}

			if tc.preparation != nil {
				tc.preparation(ch)
			}

			retc := make(chan error)
			defer close(retc)

			go func() {
				intRetc := make(chan error)
				defer close(intRetc)

				go func() {
					_, err := ch.read()

					intRetc <- err
				}()

				select {
				case <-time.After(1 * time.Second):
					return
				case retc <- <-intRetc:
				}
			}()

			select {
			case <-time.After(2 * time.Second):
				if !tc.expectTimeout {
					t.Fatal("unexpected timeout")
				}
			case err := <-retc:
				if err != tc.expectedErr {
					t.Fatalf("expected err to be %v; got %v", tc.expectedErr, err)
				}

				if tc.expectTimeout {
					t.Fatalf("expected timeout; got %v", err)
				}
			}

		})
	}

}
