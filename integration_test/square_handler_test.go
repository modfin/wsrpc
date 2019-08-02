package integration_test

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/modfin/wsrpc"
)

type squareTest struct {
	val      int
	expected int
}

func (st squareTest) Compare(target interface{}) bool {
	b, ok := target.(json.RawMessage)
	if !ok {
		fmt.Println("failed to assert json.RawMessage, should be ", reflect.TypeOf(target))
		return false
	}

	var i int
	err := json.Unmarshal(b, &i)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return i == st.expected
}

func (st squareTest) Expected() interface{} {
	return st.expected
}

func (st squareTest) Method() string {
	return "square"
}

func (st squareTest) Type() wsrpc.RequestType {
	return wsrpc.TypeCall
}

func (st squareTest) GenerateRequest() (*wsrpc.Request, error) {
	idLock.Lock()
	taskIdCounter += 1
	id := taskIdCounter
	idLock.Unlock()

	req := &wsrpc.Request{
		Id:     id,
		Method: "square",
		Type:   wsrpc.TypeCall,
		Header: wsrpc.NewHeader(),
	}
	params := struct {
		Val int `json:"val"`
	}{
		Val: st.val,
	}

	var err error
	req.Params, err = json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	return req, nil
}
