package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/modfin/wsrpc"
)

const (
	HttpClient      = "http client"
	WebSocketClient = "web socket client"

	serviceUrl = "localhost:10101"
)

type client interface {
	Type() string
	Req(...wsrpc.Request) ([]wsrpc.Response, error)
	protocol() protocol
}

type HTTPClient struct {
	clientType string
}

func (c *HTTPClient) Req(requests ...wsrpc.Request) (responses []wsrpc.Response, err error) {
	if len(requests) == 0 {
		return nil, errors.New("missing request")
	}

	job := struct {
		isBatch bool
		ofType  wsrpc.RequestType
	}{
		isBatch: len(requests) > 1,
		ofType:  requests[0].Type,
	}

	idToRequest := make(map[int]*wsrpc.Request)
	for i := range requests {
		req := &requests[i]
		idToRequest[req.Id] = req
	}

	outc := make(chan wsrpc.Response)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for {
			msg, ok := <-outc
			if !ok {
				wg.Done()
				return
			}

			responses = append(responses, msg)
		}
	}()

	activeJobs := len(requests)
	if job.isBatch && job.ofType != wsrpc.TypeStream {
		activeJobs = 1
	}

	for activeJobs > 0 {
		var data []byte
		data, err = json.Marshal(requests)
		if err != nil {
			fmt.Println(err)
			break
		}

		var resp *http.Response
		resp, err = http.Post("http://"+serviceUrl, "application/json", bytes.NewBuffer(data))
		if err != nil {
			fmt.Println("server call returned err: ", err)
			break
		}

		switch job.ofType {
		case wsrpc.TypeStream:
			var res wsrpc.Response
			err = json.NewDecoder(resp.Body).Decode(&res)
			if err != nil {
				fmt.Println("failed to decode response", err)
				break
			}

			if res.Error != nil && res.Error.Code == 205 {
				for i, req := range requests {
					if res.Id == req.Id {
						requests = append(requests[:i], requests[i+1:]...)
					}
				}

				activeJobs -= 1

				break
			}

			outc <- res

			for i, req := range requests {
				if res.Id == req.Id {
					requests[i].Header = res.Header
				}
			}

		case wsrpc.TypeCall:
			var resps []wsrpc.Response
			err = json.NewDecoder(resp.Body).Decode(&resps)
			if err != nil {
				fmt.Println("decode response: ", err)
				break
			}

			for _, res := range resps {
				outc <- res
			}

			activeJobs -= 1
		}

		if activeJobs == 0 {
			break
		}
	}

	close(outc)
	if err != nil {
		return nil, err
	}

	wg.Wait()

	return responses, nil
}

func (c *HTTPClient) Type() string {
	return c.clientType
}

func (c *HTTPClient) protocol() protocol {
	return protocolLongPoll
}

type WSClient struct {
	clientType string
}

func (c *WSClient) Req(requests ...wsrpc.Request) (responses []wsrpc.Response, err error) {
	if len(requests) < 1 {
		return nil, errors.New("missing requests")
	}
	job := struct {
		isBatch bool
		ofType  wsrpc.RequestType
	}{
		isBatch: len(requests) > 1,
		ofType:  requests[0].Type,
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+serviceUrl, nil)
	if err != nil {
		return nil, err
	}

	outc := make(chan *wsrpc.Response)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for {
			res, ok := <-outc
			if !ok {
				wg.Done()
				return
			}

			responses = append(responses, *res)
		}
	}()

	data, err := json.Marshal(requests)
	if err != nil {
		return nil, err
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return nil, err
	}

	awaitedResponses := len(requests)
	if job.ofType == wsrpc.TypeCall {
		awaitedResponses = 1
	}

	var respBuf []*wsrpc.Response
	for {
		var msgType int
		var data []byte
		msgType, data, err = conn.ReadMessage()
		if err != nil {
			break
		}
		if msgType != websocket.TextMessage {
			err = errors.New("msg not a text frame")
			break
		}

		respBuf = make([]*wsrpc.Response, 0)
		if len(data) > 0 && data[0] == '[' {
			err = json.Unmarshal(data, &respBuf)
			if err != nil {
				break
			}
		}
		if len(data) > 0 && data[0] == '{' {
			var res *wsrpc.Response
			err = json.Unmarshal(data, &res)
			if err != nil {
				break
			}

			respBuf = append(respBuf, res)
		}

		if job.ofType == wsrpc.TypeCall {
			for _, res := range respBuf {
				outc <- res
			}

			break
		}

		for _, res := range respBuf {
			if res.Error != nil && res.Error.Code == 205 {
				awaitedResponses -= 1

				continue
			}

			outc <- res
		}
		if awaitedResponses == 0 {
			break
		}

	}

	close(outc)
	if err != nil {
		return nil, err
	}

	// Possibly redundant synchronization,
	// close(outc) should be run before this point
	wg.Wait()

	return responses, nil
}

func (c *WSClient) Type() string {
	return c.clientType
}

func (c *WSClient) protocol() protocol {
	return protocolWebSocket
}
