package wsrpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Error contains all the necessary info when handling or returning an error in the wsrpc server.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	err     error           `json:"-"`
}

var (
	errMissingRequestId = errors.New("missing request id")
	errUnsupportedFrame = errors.New("frame content is not text")
	errMissingRequest   = errors.New("missing request")
	errMixedTypes       = errors.New("mixed types is not allowed")
	errMethodNotFound   = errors.New("method not found")
)

// Error is a stringer
func (r Error) Error() string {
	return fmt.Sprintf("wsrpc: %v, message=%s", r.Code, r.Message)
}

// TypeNotFoundError is called when an invalid request type is requested.
func TypeNotFoundError(t RequestType) *Error {
	return &Error{
		Code:    -32600,
		Message: fmt.Sprintf("type not found: %s", t),
	}
}

// MethodNotFoundError is called when a non existing method is requested.
func MethodNotFoundError(method string) *Error {
	return &Error{
		Code:    -32601,
		Message: fmt.Sprintf(errMethodNotFound.Error()+": %s", method),
	}
}

// IdIsRequiredError is called when an id is missing from a request.
func IdIsRequiredError() *Error {
	return &Error{
		Code:    -32600,
		Message: "invalid request: id must be provided",
	}
}

// ServerError repackages any regular error message into a wsrpc error which can be passed to a response.
func ServerError(outpErr error) *Error {
	errData, err := json.Marshal(outpErr)
	if err != nil {
		errData = json.RawMessage(err.Error())
	}
	return &Error{
		Code:    -32000,
		Message: fmt.Sprintf("server error: %s", outpErr.Error()),
		Data:    errData,
	}
}

// EOF is used to denote end of contents in stream requests.
func EOF() *Error {
	return &Error{
		Code:    205,
		Message: "EOF",
	}
}
