package wsrpc

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestServer starts r behind httptest and drains r.errc, which is otherwise only drained by Start.
func newTestServer(t *testing.T, r *Router) (*httptest.Server, <-chan error) {
	errs := make(chan error, 100)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case err := <-r.errc:
				select {
				case errs <- err:
				default:
				}
			case <-done:
				return
			}
		}
	}()

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		close(done)
	})

	return srv, errs
}

func dialTestServer(t *testing.T, srv *httptest.Server) *websocket.Conn {
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn
}

func readResponse(t *testing.T, conn *websocket.Conn) Response {
	err := conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var res Response
	err = json.Unmarshal(data, &res)
	if err != nil {
		t.Fatalf("unmarshal response %q: %v", data, err)
	}

	return res
}

func TestRouter_InvalidFrameDoesNotKillServer(t *testing.T) {
	r := NewRouter()
	r.SetHandler("ping", func(ctx Context) error {
		ctx.Response().Result = json.RawMessage(`"pong"`)
		return nil
	})

	srv, _ := newTestServer(t, r)
	conn := dialTestServer(t, srv)

	frames := []string{
		`not json`,
		`[]`,
		`{"method":"ping","type":"CALL"`,
		`[{"id":1,"method":"ping","type":"CALL"},{"id":2,"method":"ping","type":"STREAM"}]`,
	}
	for _, f := range frames {
		err := conn.WriteMessage(websocket.TextMessage, []byte(f))
		if err != nil {
			t.Fatalf("write %q: %v", f, err)
		}
	}

	err := conn.WriteMessage(websocket.TextMessage, []byte(`{"id":42,"method":"ping","type":"CALL"}`))
	if err != nil {
		t.Fatalf("write valid request: %v", err)
	}

	res := readResponse(t, conn)
	if res.Id != 42 || string(res.Result) != `"pong"` || res.Error != nil {
		t.Fatalf("expected pong for id 42; got %+v", res)
	}
}

func TestRouter_HandlerPanicIsRecovered(t *testing.T) {
	r := NewRouter()
	r.SetHandler("boom", func(ctx Context) error {
		panic("boom")
	})
	r.SetHandler("ping", func(ctx Context) error {
		ctx.Response().Result = json.RawMessage(`"pong"`)
		return nil
	})

	srv, errs := newTestServer(t, r)
	conn := dialTestServer(t, srv)

	err := conn.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"boom","type":"CALL"}`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	res := readResponse(t, conn)
	if res.Id != 1 || res.Error == nil || !strings.Contains(res.Error.Message, "boom") {
		t.Fatalf("expected error response for id 1; got %+v", res)
	}

	select {
	case err := <-errs:
		if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "goroutine") {
			t.Fatalf("expected panic error with stack; got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected panic to be reported on errc")
	}

	err = conn.WriteMessage(websocket.TextMessage, []byte(`{"id":2,"method":"ping","type":"CALL"}`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	res = readResponse(t, conn)
	if res.Id != 2 || string(res.Result) != `"pong"` {
		t.Fatalf("expected pong for id 2; got %+v", res)
	}
}

func FuzzCreateBatch(f *testing.F) {
	seeds := []string{
		``,
		`not json`,
		`[]`,
		`{}`,
		`null`,
		`[null]`,
		`{"id":1,"method":"ping","type":"CALL"}`,
		`{"jobId":"8f6c2b1e-6a43-4f0e-9a8e-0c1d2e3f4a5b","method":"ping","type":"STREAM"}`,
		`[{"id":1,"method":"ping","type":"CALL"},{"id":2,"method":"ping","type":"STREAM"}]`,
		`[{"id":1,"header":{"a":1},"params":{"x":[1,2]}}]`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		b, err := createBatch(data, httptest.NewRequest("GET", "/", nil))
		if err != nil {
			if b != nil {
				t.Fatalf("got both batch and error %v", err)
			}
			return
		}
		if b == nil {
			t.Fatal("got (nil, nil)")
		}
		if b.channel == nil || len(b.jobs) == 0 {
			t.Fatalf("batch is not usable: %+v", b)
		}
		for i := range b.jobs {
			if b.jobs[i].request == nil {
				t.Fatalf("job %d has nil request", i)
			}
		}
		b.kill()
	})
}
