package orderack

import (
	"embed"
	"iter"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
Options parameterize one private add_order acknowledgement frame.
*/
type Options struct {
	ReqID   int64
	OrderID string
	Success bool
}

/*
Frame builds one order acknowledgement payload for broker position tests.
*/
func Frame(options Options) []byte {
	raw, err := fixtureFiles.ReadFile("fixtures/ack.json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "orderack fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "orderack fixture decode failed", err))
	}

	payload["req_id"] = options.ReqID
	payload["success"] = options.Success

	result, ok := payload["result"].(map[string]any)

	if !ok {
		panic(errnie.Err(errnie.Validation, "orderack fixture result missing", nil))
	}

	result["order_id"] = options.OrderID

	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "orderack fixture encode failed", err))
	}

	return encoded
}

/*
Fixture replays one ordered list of order acknowledgements.
*/
type Fixture struct {
	payloads [][]byte
}

/*
NewFixture builds an acknowledgement sequence from explicit options.
*/
func NewFixture(options ...Options) *Fixture {
	payloads := make([][]byte, len(options))

	for index, option := range options {
		payloads[index] = Frame(option)
	}

	return &Fixture{payloads: payloads}
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, payload := range fixture.payloads {
			if !yield(payload) {
				return
			}
		}
	}
}
