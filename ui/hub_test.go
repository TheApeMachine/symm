package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

const hubLifecycleTestTimeout = 5 * time.Second

func TestHubRun(t *testing.T) {
	Convey("Given a running dashboard hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		hub := NewHub(ctx, nil, nil, nil, nil, nil)
		hub.listenAddr = "127.0.0.1:0"
		listening := make(chan struct{})
		done := make(chan error, 1)
		hub.app.Hooks().OnListen(func(fiber.ListenData) error {
			close(listening)
			return nil
		})

		go func() { done <- hub.Run() }()

		select {
		case <-listening:
		case <-time.After(hubLifecycleTestTimeout):
			t.Fatal("hub did not start listening")
		}

		cancel()

		Convey("It should stop listening when its context is canceled", func() {
			select {
			case err := <-done:
				So(err, ShouldBeNil)
			case <-time.After(hubLifecycleTestTimeout):
				t.Fatal("hub did not stop after context cancellation")
			}
		})
	})
}

func TestExpectedDashboardWriteClosure(t *testing.T) {
	Convey("Given the close sentinel returned by the underlying connection", t, func() {
		err := fmt.Errorf("dashboard write: %w", websocket.ErrCloseSent)

		Convey("It should classify the completed close handshake as expected", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeTrue)
		})
	})

	Convey("Given an unrelated write failure", t, func() {
		err := errors.New("unexpected write failure")

		Convey("It should preserve the failure for logging", func() {
			So(expectedDashboardWriteClosure(err), ShouldBeFalse)
		})
	})
}

func TestSplitDashboardFrame(t *testing.T) {
	Convey("Given a strategy frame larger than one configured message", t, func() {
		decisions := []*wire.DecisionT{
			{Symbol: "BTC/USD"},
			{Symbol: "ETH/USD"},
			{Symbol: "SOL/USD"},
			{Symbol: "XRP/USD"},
		}
		oneDecision := &wire.FrameT{
			Type: wire.FrameStrategyFrame,
			Value: &wire.StrategyFrameT{
				Outcome:   "accumulating",
				Decisions: decisions[:1],
			},
		}
		encoded := telemetry.EncodeBatch([]*types.UIFrame{oneDecision})
		maxMessageBytes := len(encoded.Bytes)
		encoded.Release()
		frame := &wire.FrameT{
			Type: wire.FrameStrategyFrame,
			Value: &wire.StrategyFrameT{
				Outcome:   "accumulating",
				Decisions: decisions,
			},
		}

		frames, err := splitDashboardFrame(frame, maxMessageBytes)

		Convey("It should preserve every decision in order without exceeding the limit", func() {
			So(err, ShouldBeNil)
			var symbols []string

			for _, splitFrame := range frames {
				batch := telemetry.EncodeBatch([]*types.UIFrame{splitFrame})
				So(len(batch.Bytes), ShouldBeLessThanOrEqualTo, maxMessageBytes)
				batch.Release()
				strategy := splitFrame.Value.(*wire.StrategyFrameT)

				for _, decision := range strategy.Decisions {
					symbols = append(symbols, decision.Symbol)
				}
			}

			So(symbols, ShouldResemble, []string{
				"BTC/USD",
				"ETH/USD",
				"SOL/USD",
				"XRP/USD",
			})
		})
	})
}

func BenchmarkSplitDashboardFrame(b *testing.B) {
	decisions := make([]*wire.DecisionT, 512)

	for index := range decisions {
		decisions[index] = &wire.DecisionT{Symbol: fmt.Sprintf("symbol-%d", index)}
	}

	frame := &wire.FrameT{
		Type: wire.FrameStrategyFrame,
		Value: &wire.StrategyFrameT{
			Outcome:   "accumulating",
			Decisions: decisions,
		},
	}
	maxMessageBytes := len(telemetry.Encode(frame)) / 2
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		frames, err := splitDashboardFrame(frame, maxMessageBytes)

		if err != nil {
			b.Fatal(err)
		}

		if len(frames) < 2 {
			b.Fatal("strategy frame was not split")
		}
	}
}
