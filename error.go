package wsrpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	err     error           `json:"-"`
}

var (
	errMissingRequestId = errors.New("missing request id")
	errUnsupportedFrame = errors.New("frame content is not text")
	ErrMissingParams    = errors.New("missing function parameters")
	errMissingRequest   = errors.New("missing request")
	errMixedTypes       = errors.New("mixed types is not allowed")
	errMethodNotFound   = errors.New("method not found")
)

func (r Error) Error() string {
	return fmt.Sprintf("wsrpc: %v, message=%s", r.Code, r.Message)
}

func TypeNotFoundError(t RequestType) *Error {
	return &Error{
		Code:    -32600,
		Message: fmt.Sprintf("type not found: %s", t),
	}
}

func MethodNotFoundError(method string) *Error {
	return &Error{
		Code:    -32601,
		Message: fmt.Sprintf(errMethodNotFound.Error()+": %s", method),
	}
}

func IdIsRequiredError() *Error {
	return &Error{
		Code:    -32600,
		Message: "invalid request: id must be provided",
	}
}

func ServerError(message string) *Error {
	return &Error{
		Code:    -32000,
		Message: fmt.Sprintf("server error: %s", message),
	}
}

func EOF() *Error {
	return &Error{
		Code:    205,
		Message: "EOF",
	}
}
