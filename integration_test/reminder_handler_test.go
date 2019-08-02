package integration_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/modfin/wsrpc"
)

type reminderTest struct {
	alarmTime time.Time
	msg       string
}

func (rt reminderTest) Compare(target interface{}) bool {
	data, ok := target.(json.RawMessage)
	if !ok {
		fmt.Println("failed to assert json.RawMessage, should be ", reflect.TypeOf(target))
		return false
	}

	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return s == rt.msg
}

func (rt reminderTest) Expected() interface{} {
	return string(rt.msg)
}

func (rt reminderTest) Method() string {
	return "reminder"
}

func (rt reminderTest) Type() wsrpc.RequestType {
	return wsrpc.TypeStream
}
func (rt reminderTest) GenerateRequest() (*wsrpc.Request, error) {
	idLock.Lock()
	taskIdCounter += 1
	id := taskIdCounter
	idLock.Unlock()

	req := &wsrpc.Request{
		Id:     id,
		Method: "reminder",
		Type:   wsrpc.TypeStream,
		Header: wsrpc.NewHeader(),
	}
	params := struct {
		When time.Time `json:"time"`
		Msg  string    `json:"msg"`
	}{
		When: rt.alarmTime,
		Msg:  rt.msg,
	}

	var err error
	req.Params, err = json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	return req, nil
}
