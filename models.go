package wsrpc

import (
	"encoding/json"
)

// RequestType is used to denote what kind of job is being requested
type RequestType string

const (
	// TypeStream refers to jobs of streaming character.
	TypeStream RequestType = `STREAM`
	// TypeCall refers to jobs of on demand requests character.
	TypeCall RequestType = `CALL`
)

// Response is serialized and passed back to the requester.
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

// Request contains information about the job requested by the client.
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
