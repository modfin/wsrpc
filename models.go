package wsrpc

import (
	"encoding/json"

	"github.com/google/uuid"
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
	Id     int             `json:"id"` // Legacy
	JobId  uuid.UUID       `json:"jobId"`
	Result json.RawMessage `json:"result"`
	Header Headers         `json:"header"`
	Error  *Error          `json:"error"`
}

func newResponse(id int, jobId uuid.UUID, err *Error) *Response {
	return &Response{
		Id:     id,
		JobId:  jobId,
		Error:  err,
		Header: NewHeader(),
	}
}

// Request contains information about the job requested by the client.
type Request struct {
	Id     int             `json:"id"` // Legacy
	JobId  uuid.UUID       `json:"jobId"`
	Method string          `json:"method"`
	Type   RequestType     `json:"type"`
	Params json.RawMessage `json:"params"`
	Header Headers         `json:"header"`
}
type RequestTarget Request

func (r *Request) MarshalJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *Request) UnmarshalJSON(data []byte) error {
	var rt *RequestTarget
	err := json.Unmarshal(data, &rt)
	if err != nil {
		return err
	}

	*r = Request(*rt)

	if r.JobId == uuid.Nil {
		r.JobId = uuid.New()
	}

	return nil
}

func newRequest() Request {
	return Request{
		Header: NewHeader(),
	}
}
