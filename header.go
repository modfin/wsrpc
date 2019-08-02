package wsrpc

import (
	"github.com/modfin/kv"
)

type Headers map[string]interface{}

func NewHeader() Headers {
	return make(map[string]interface{})
}

func (h Headers) Set(key string, value interface{}) {
	h[key] = value
}

func (h Headers) Get(key string) *kv.KV {
	value, exists := h[key]
	if !exists {
		return kv.New("", nil)
	}

	return kv.New(key, value)
}
