package wsrpc

import (
	"encoding/json"
)

type RequestType string

const (
	TypeStream RequestType = `STREAM`
	TypeCall   RequestType = `CALL`
)

type Response struct {
	Id     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Header Headers         `json:"header"`
	Error  *Error          `json:"error"`
}

func newResponse(id int, err *Error) *Response {
	return &Response{
		Id:     id,
		Error:  err,
		Header: NewHeader(),
	}
}

type Request struct {
	Id     int             `json:"id"`
	Method string          `json:"method"`
	Type   RequestType     `json:"type"`
	Params json.RawMessage `json:"params"`
	Header Headers         `json:"header"`
}

func newRequest() Request {
	return Request{
		Header: NewHeader(),
	}
}
