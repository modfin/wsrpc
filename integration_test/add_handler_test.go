package integration_test

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/modfin/wsrpc"
)

type addTest struct {
	A        int
	B        int
	expected int
}

func (at addTest) Compare(target interface{}) bool {
	b, ok := target.(json.RawMessage)
	if !ok {
		fmt.Println("failed to assert json.RawMessage, should be ", reflect.TypeOf(target))
		return false
	}

	var i int
	err := json.Unmarshal([]byte(b), &i)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return i == at.expected
}

func (at addTest) Expected() interface{} {
	return at.expected
}

func (at addTest) Method() string {
	return "add"
}

func (at addTest) Type() wsrpc.RequestType {
	return wsrpc.TypeCall
}

func (at addTest) GenerateRequest() (*wsrpc.Request, error) {
	idLock.Lock()
	taskIdCounter += 1
	id := taskIdCounter
	idLock.Unlock()

	req := &wsrpc.Request{
		Id:     id,
		Method: "add",
		Type:   wsrpc.TypeCall,
		Header: wsrpc.NewHeader(),
	}
	req.Header.Set("A", at.A)
	req.Header.Set("B", at.B)

	return req, nil
}
