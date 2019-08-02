package integration_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/modfin/wsrpc"
)

var (
	taskIdCounter int
	idLock        = sync.Mutex{}

	router  *wsrpc.Router
	clients []client
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	shutdown()
	os.Exit(code)
}

func setup() {
	router = setupRouter()

	go router.Start(":10101")

	time.Sleep(500 * time.Millisecond)

	clients = []client{
		&HTTPClient{HttpClient},
		&WSClient{WebSocketClient},
	}

}

func shutdown() {

}

var tests = struct {
	add       []addTest
	square    []squareTest
	countdown []countdownTest
	reminder  []reminderTest
}{
	add: []addTest{
		{1, 2, 3},
		{3, 4, 7},
		{5, 5, 10},
	},
	square: []squareTest{
		{0, 0},
		{-1, 1},
		{2, 4},
	},
	countdown: []countdownTest{
		{0, nil},
		{4, []int{1, 2, 3, 4}},
	},
	reminder: []reminderTest{
		{time.Time{}, "Hello Gopher!"},
		{time.Now().Add(2 * time.Second), "Hello future Gopher!"},
	},
}

type protocol string

const (
	protocolWebSocket protocol = "webSocket"
	protocolLongPoll  protocol = "longPoll"
)

type testData struct {
	data wsrpcTest
}
type testRow struct {
	name     string
	method   string
	testData []testData
	batch    bool
	cl       client
}

type wsrpcTest interface {
	Method() string
	Type() wsrpc.RequestType
	Compare(interface{}) bool
	Expected() interface{}
	GenerateRequest() (*wsrpc.Request, error)
}

func buildTestTable() []testRow {
	var testRows []testRow

	for _, client := range clients {
		for _, batch := range []bool{true, false} {
			for _, method := range []string{"add", "square", "countdown", "reminder"} {
				tr := testRow{
					cl:     client,
					batch:  batch,
					method: method,
				}

				var handlerType wsrpc.RequestType
				switch method {
				case "add":
					handlerType = wsrpc.TypeCall
					for _, t := range tests.add {
						tr.testData = append(tr.testData, testData{data: t})
					}
				case "square":
					handlerType = wsrpc.TypeCall
					for _, t := range tests.square {
						tr.testData = append(tr.testData, testData{data: t})
					}
				case "countdown":
					handlerType = wsrpc.TypeStream
					for _, t := range tests.countdown {
						tr.testData = append(tr.testData, testData{data: t})
					}
				case "reminder":
					handlerType = wsrpc.TypeStream
					for _, t := range tests.reminder {
						tr.testData = append(tr.testData, testData{data: t})
					}
				default:
					log.Fatal("There's probably a pig flying outside as well")
				}

				tr.name = fmt.Sprintf("%v %v %v batch=%v",
					tr.cl.protocol(),
					handlerType,
					tr.method,
					tr.batch,
				)

				testRows = append(testRows, tr)
			}
		}
	}

	return testRows
}

func TestIntegration(t *testing.T) {
	tt := buildTestTable()

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			idToTest := make(map[int]wsrpcTest)
			var reqs []wsrpc.Request

			for _, td := range tc.testData {
				req, err := td.data.GenerateRequest()
				if err != nil {
					log.Fatalf("failed to generate wsrpc request: %+v", err)
				}

				idToTest[req.Id] = td.data

				reqs = append(reqs, *req)
			}

			var batches [][]wsrpc.Request
			if tc.batch {
				batches = append(batches, reqs)
			}
			if !tc.batch {
				for _, req := range reqs {
					batches = append(batches, []wsrpc.Request{req})
				}
			}

			var responses []wsrpc.Response
			for _, req := range batches {
				res, err := tc.cl.Req(req...)
				if err != nil {
					t.Fatalf("failed to execute wsrpc request")
				}

				responses = append(responses, res...)
			}

			for id, test := range idToTest {
				var aggregatedResult []json.RawMessage
				for _, res := range responses {
					if res.Id != id {
						continue
					}

					aggregatedResult = append(aggregatedResult, res.Result)
				}

				var result interface{}
				if len(aggregatedResult) == 0 && test.Expected() != nil {
					t.Errorf("no results to test were returned for request %v", id)
					continue
				}
				if len(aggregatedResult) == 1 {
					result = aggregatedResult[0]
				}
				if len(aggregatedResult) > 1 {
					result = aggregatedResult
				}

				testPassed := test.Compare(result)
				if !testPassed {
					t.Errorf("test failed to pass validation. expected %+v; got %+v", test.Expected(), string(result.(json.RawMessage)))
				}
			}
		})
	}
}
