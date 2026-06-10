package ui

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func TestHubDrainCoalescesUI(t *testing.T) {
	Convey("Given repeated gauge frames for one source", t, func() {
		viper.Set("system.queue.buffer", 64)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 4, nil)

		hub := &Hub{
			ctx: ctx,
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelUI},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelUI, "ui:test"),
				},
			),
		}

		for index := 0; index < 8; index++ {
			err := hub.bus.Send(internal.ChannelUI, "gauge", map[string]any{
				"source":     "cvd",
				"confidence": float64(index),
			})

			So(err, ShouldBeNil)
		}

		Convey("It should keep only the latest frame per coalesce key", func() {
			first, receiveErr := hub.bus.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)
			So(first, ShouldNotBeNil)

			pending := make(map[string]any)

			key, value, ok := hub.prepareUIFrame(first)

			So(ok, ShouldBeTrue)
			pending[key] = value

			for {
				row, pollErr := hub.bus.Poll(internal.ChannelUI)

				if pollErr != nil || row == nil {
					break
				}

				nextKey, nextValue, nextOK := hub.prepareUIFrame(row)

				if !nextOK {
					continue
				}

				pending[nextKey] = nextValue
			}

			So(len(pending), ShouldEqual, 1)

			frame, ok := pending["gauge:cvd"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(frame["confidence"], ShouldEqual, 7.0)
		})
	})
}

func BenchmarkHubPrepareUIFrame(b *testing.B) {
	hub := &Hub{}
	row := &qpool.QValue[any]{
		Type: "gauge",
		Value: map[string]any{
			"source":     "cvd",
			"confidence": 0.75,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = hub.prepareUIFrame(row)
	}
}
