package wsrpc

import (
	"github.com/modfin/kv"
)

// Headers is the wsrpc equivalent of HTTP headers. They work much the same way.
type Headers map[string]interface{}

// NewHeader generates a new headers object.
func NewHeader() Headers {
	return make(map[string]interface{})
}

// Set puts a key value pair on the headers object.
func (h Headers) Set(key string, value interface{}) {
	h[key] = value
}

// Get collects a key value pair from a headers object.
func (h Headers) Get(key string) *kv.KV {
	value, exists := h[key]
	if !exists {
		return kv.New("", nil)
	}

	return kv.New(key, value)
}
