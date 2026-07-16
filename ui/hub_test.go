package ui

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewHub(t *testing.T) {
	Convey("Given the dashboard thesis", t, func() {
		channel := make(chan []byte, 1)
		thesis := types.NewThesis(channel)

		hub, err := NewHub(context.Background(), nil, nil, thesis, channel)

		Convey("It should retain the thesis used by websocket publication", func() {
			So(err, ShouldBeNil)
			So(hub.Thesis(), ShouldEqual, thesis)
			focus, source := thesis.UIProjection()
			So(focus, ShouldEqual, "BTC/USD")
			So(source, ShouldEqual, types.SourceFluid)
		})
	})
}

func TestHubCurrentFrame(t *testing.T) {
	Convey("Given queued snapshots accumulated behind the websocket writer", t, func() {
		channel := make(chan []byte, 4)
		hub := &Hub{Messages: channel}
		channel <- []byte(`{"tick":{"count":2}}`)
		channel <- []byte(`{"measurements":[{"symbol":"BTC/USD"}]}`)
		channel <- []byte(`{"tick":{"count":3}}`)

		current, err := hub.currentFrame([]byte(`{"tick":{"count":1}}`))
		frame := struct {
			Tick         map[string]int `json:"tick"`
			Measurements []any          `json:"measurements"`
		}{}
		decodeErr := sonic.Unmarshal(current, &frame)

		Convey("It should retain only the newest value for each frame key", func() {
			So(err, ShouldBeNil)
			So(decodeErr, ShouldBeNil)
			So(frame.Tick["count"], ShouldEqual, 3)
			So(frame.Measurements, ShouldHaveLength, 1)
			So(len(channel), ShouldEqual, 0)
		})
	})
}

func TestHubSetProjection(t *testing.T) {
	Convey("Given an active dashboard thesis", t, func() {
		channel := make(chan []byte, 1)
		thesis := types.NewThesis(channel)
		hub, err := NewHub(context.Background(), nil, nil, thesis, channel)
		So(err, ShouldBeNil)

		hub.SetProjection("ETH/USD", types.SourceHawkes)
		next := types.NewThesis(channel)
		hub.SetThesis(next)

		Convey("It should carry the operator scope into subsequent ticks", func() {
			focus, source := next.UIProjection()
			So(focus, ShouldEqual, "ETH/USD")
			So(source, ShouldEqual, types.SourceHawkes)
		})
	})
}

func BenchmarkHubCurrentFrame(b *testing.B) {
	frames := [][]byte{
		[]byte(`{"tick":{"count":1}}`),
		[]byte(`{"measurements":[{"symbol":"BTC/USD","raw":1}]}`),
		[]byte(`{"tick":{"count":2}}`),
		[]byte(`{"lifecycle":{"BTC/USD":"observing"}}`),
	}
	hub := &Hub{}
	b.ReportAllocs()

	for b.Loop() {
		hub.Messages = make(chan []byte, len(frames)-1)

		for _, frame := range frames[1:] {
			hub.Messages <- frame
		}

		if _, err := hub.currentFrame(frames[0]); err != nil {
			b.Fatal(err)
		}
	}
}
