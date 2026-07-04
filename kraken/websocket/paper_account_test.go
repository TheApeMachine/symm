package websocket

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPaperAccountSync(t *testing.T) {
	Convey("Given paper account balance and order snapshots", t, func() {
		configurePaperAccountBuffer(t)

		runner := newFakeRunner(map[string][][]byte{
			"paper balance": {[]byte(`{"balances":{"USD":{"available":200.18,"reserved":0,"total":200.18}},"mode":"paper"}`)},
			"paper orders":  {[]byte(`{"count":0,"mode":"paper","open_orders":[]}`)},
			"paper history": {[]byte(`{"mode":"paper","trades":[]}`)},
		})

		account := NewPaperAccountWithRunner(context.Background(), runner)
		frames := account.Observe()

		Convey("When Sync runs", func() {
			err := account.Sync()
			balances := waitFrame(t, frames, "balances")
			orders := waitFrame(t, frames, "orders")

			Convey("It should publish balance and order snapshots", func() {
				So(err, ShouldBeNil)
				So(balances["type"], ShouldEqual, "snapshot")
				So(orders["type"], ShouldEqual, "snapshot")
				So(orders["count"], ShouldEqual, 0)

				data := balances["data"].([]map[string]any)
				row := data[0]

				So(row["asset"], ShouldEqual, "USD")
				So(row["balance"], ShouldEqual, 200.18)
			})
		})
	})

	Convey("Given paper account history with one already seen execution", t, func() {
		configurePaperAccountBuffer(t)

		runner := newFakeRunner(map[string][][]byte{
			"paper balance": {
				[]byte(`{"balances":{"USD":{"available":200,"reserved":0,"total":200}},"mode":"paper"}`),
				[]byte(`{"balances":{"USD":{"available":201,"reserved":0,"total":201}},"mode":"paper"}`),
			},
			"paper orders": {
				[]byte(`{"count":0,"mode":"paper","open_orders":[]}`),
				[]byte(`{"count":0,"mode":"paper","open_orders":[]}`),
			},
			"paper history": {
				[]byte(`{"mode":"paper","trades":[{"id":"PAPER-00001","order_id":"PAPER-00000","pair":"ETHUSD","price":100,"side":"buy","status":"filled","time":"2026-07-04T10:00:00Z","volume":1,"cost":100,"fee":0.26}]}`),
				[]byte(`{"mode":"paper","trades":[{"id":"PAPER-00001","order_id":"PAPER-00000","pair":"ETHUSD","price":100,"side":"buy","status":"filled","time":"2026-07-04T10:00:00Z","volume":1,"cost":100,"fee":0.26},{"id":"PAPER-00002","order_id":"PAPER-00002","pair":"NEARUSD","price":2,"side":"sell","status":"filled","time":"2026-07-04T10:01:00Z","volume":20,"cost":40,"fee":0.104}]}`),
			},
		})

		account := NewPaperAccountWithRunner(context.Background(), runner)
		frames := account.Observe()

		Convey("When Sync runs twice", func() {
			firstErr := account.Sync()
			drainFrames(frames)

			secondErr := account.Sync()
			execution := waitFrame(t, frames, "executions")
			rows := execution["data"].([]map[string]any)
			row := rows[0]

			Convey("It should publish only the new execution", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(row["exec_id"], ShouldEqual, "PAPER-00002")
				So(row["symbol"], ShouldEqual, "NEAR/USD")
				So(fmt.Sprint(row["order_qty"]), ShouldEqual, "20")
			})
		})
	})
}

type fakeRunner struct {
	outputs map[string][][]byte
	calls   map[string]int
}

func newFakeRunner(outputs map[string][][]byte) *fakeRunner {
	return &fakeRunner{
		outputs: outputs,
		calls:   make(map[string]int),
	}
}

func (runner *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	sequence, ok := runner.outputs[key]
	if !ok || len(sequence) == 0 {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}

	index := runner.calls[key]
	runner.calls[key]++
	if index >= len(sequence) {
		index = len(sequence) - 1
	}

	return sequence[index], nil
}

func configurePaperAccountBuffer(t *testing.T) {
	t.Helper()

	previous := viper.GetInt("system.websocket.channel.buffer")
	viper.Set("system.websocket.channel.buffer", 8)
	t.Cleanup(func() {
		viper.Set("system.websocket.channel.buffer", previous)
	})
}

func waitFrame(t *testing.T, frames <-chan map[string]any, channel string) map[string]any {
	t.Helper()

	timeout := time.After(time.Second)
	for {
		select {
		case frame := <-frames:
			if frame["channel"] == channel {
				return frame
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s frame", channel)
		}
	}
}

func drainFrames(frames <-chan map[string]any) {
	for {
		select {
		case <-frames:
		default:
			return
		}
	}
}
