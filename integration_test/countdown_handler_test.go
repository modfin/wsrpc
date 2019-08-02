package integration_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/modfin/wsrpc"
)

type countdownTest struct {
	number         int
	expectedSeries []int
}

func (ct countdownTest) Compare(target interface{}) bool {
	if target == nil && ct.expectedSeries == nil {
		return true
	}

	dataSet, ok := target.([]json.RawMessage)
	if !ok {
		data, ok := target.(json.RawMessage)
		if ok {
			var i int
			err := json.Unmarshal(data, &i)
			if err != nil {
				fmt.Println("failed to assert json.RawMessage, should be ", reflect.TypeOf(target))
				return false
			}

			return len(ct.expectedSeries) == 1 && ct.expectedSeries[0] == i
		}

		fmt.Println("failed to assert []json.RawMessage, should be ", reflect.TypeOf(target))
		return false
	}

	if len(dataSet) != len(ct.expectedSeries) {
		fmt.Println("result and expected result have different lengths")

		return false
	}

	resultSeries := make([]int, len(dataSet))
	for i, data := range dataSet {
		err := json.Unmarshal(data, &resultSeries[i])
		if err != nil {
			fmt.Println(err)
			return false
		}
	}

	sort.Ints(resultSeries)

	if len(resultSeries) == 1 {
		return resultSeries[0] == ct.expectedSeries[0]
	}

	for i, res := range resultSeries {
		if res != ct.expectedSeries[i] {
			return false
		}
	}

	return true
}

func (ct countdownTest) Expected() interface{} {
	if ct.expectedSeries == nil {
		return nil
	}

	return ct.expectedSeries
}

func (ct countdownTest) Method() string {
	return "countdown"
}

func (ct countdownTest) Type() wsrpc.RequestType {
	return wsrpc.TypeStream
}

func (ct countdownTest) GenerateRequest() (*wsrpc.Request, error) {
	idLock.Lock()
	taskIdCounter += 1
	id := taskIdCounter
	idLock.Unlock()

	req := &wsrpc.Request{
		Id:     id,
		Method: "countdown",
		Type:   wsrpc.TypeStream,
		Header: wsrpc.NewHeader(),
	}
	req.Header.Set("state", ct.number)

	return req, nil
}
