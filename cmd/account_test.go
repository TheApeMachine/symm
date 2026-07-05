package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	dashboard "github.com/theapemachine/symm/ui"
)

type accountBridgeObserver struct {
	frames []map[string]any
}

func (observer *accountBridgeObserver) Observe(frame map[string]any) error {
	observer.frames = append(observer.frames, frame)
	return nil
}

type accountBridgePublisher struct {
	messages []dashboard.Message
}

func (publisher *accountBridgePublisher) Publish(message dashboard.Message) error {
	publisher.messages = append(publisher.messages, message)
	return nil
}

type accountBridgeSink struct {
	observed  int
	published int
}

func (sink *accountBridgeSink) Observe(frame map[string]any) error {
	sink.observed++
	return nil
}

func (sink *accountBridgeSink) Publish(message dashboard.Message) error {
	sink.published++
	return nil
}

func TestAccountBridgePublish(testingTB *testing.T) {
	Convey("Given an account bridge with a typed dashboard publisher", testingTB, func() {
		observer := &accountBridgeObserver{}
		publisher := &accountBridgePublisher{}
		bridge := &accountBridge{
			observer:  observer,
			publisher: publisher,
		}

		Convey("When a Kraken balance frame carries generic row objects", func() {
			err := bridge.publish(map[string]any{
				"channel":   "balances",
				"count":     "1",
				"timestamp": "2026-07-04T21:00:00Z",
				"data": []any{
					map[string]any{"asset": "USD", "balance": 200.0},
				},
			})

			Convey("Then it publishes balances without an account envelope", func() {
				So(err, ShouldBeNil)
				So(observer.frames, ShouldHaveLength, 1)
				So(publisher.messages, ShouldHaveLength, 1)
				So(publisher.messages[0].Balances, ShouldNotBeNil)
				So(publisher.messages[0].Balances.Rows[0]["asset"], ShouldEqual, "USD")
				So(publisher.messages[0].Balances.Count, ShouldEqual, 1)
				So(publisher.messages[0].Orders, ShouldBeNil)
				So(publisher.messages[0].Executions, ShouldBeNil)
			})
		})

		Convey("When an unsupported channel arrives", func() {
			err := bridge.publish(map[string]any{
				"channel": "subscriptionStatus",
				"data":    []map[string]any{},
			})

			Convey("Then it fails before observing or publishing", func() {
				So(err, ShouldNotBeNil)
				So(observer.frames, ShouldHaveLength, 0)
				So(publisher.messages, ShouldHaveLength, 0)
			})
		})
	})
}

func BenchmarkAccountBridgePublish(benchmarkTB *testing.B) {
	sink := &accountBridgeSink{}
	bridge := &accountBridge{
		observer:  sink,
		publisher: sink,
	}
	frame := map[string]any{
		"channel":   "balances",
		"count":     "1",
		"timestamp": "2026-07-04T21:00:00Z",
		"data": []any{
			map[string]any{"asset": "USD", "balance": 200.0},
		},
	}

	benchmarkTB.ReportAllocs()
	for index := 0; index < benchmarkTB.N; index++ {
		if err := bridge.publish(frame); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
