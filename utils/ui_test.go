package utils

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestPublishCounters(t *testing.T) {
	Convey("Given a saturated replaceable-state channel", t, func() {
		ui := make(chan []byte, 1)
		ui <- []byte("existing")
		beforeSent, beforeDropped := PublishCounters()
		Publish(ui, datura.NewMap("ticker", "new"))
		sent, dropped := PublishCounters()

		Convey("It should count the dropped frame", func() {
			So(sent, ShouldEqual, beforeSent)
			So(dropped, ShouldEqual, beforeDropped+1)
		})
	})
}

func TestPublish(t *testing.T) {
	Convey("Given a saturated replaceable-state channel", t, func() {
		ui := make(chan []byte, 1)
		ui <- []byte("existing")

		Publish(ui, datura.NewMap("ticker", "new"))

		Convey("It should retain the already queued frame without blocking", func() {
			So(<-ui, ShouldResemble, []byte("existing"))
		})
	})
}

func TestPublishPriority(t *testing.T) {
	Convey("Given a saturated lifecycle-state channel", t, func() {
		ui := make(chan []byte, 1)
		ui <- []byte(`{"measurements":[]}`)

		PublishPriority(ui, datura.NewMap(
			"activity", datura.NewMap("planner", "running"),
		))
		PublishPriority(ui, datura.NewMap(
			"activity", datura.NewMap("planner", "done"),
		))

		Convey("It should retain the latest transition without blocking", func() {
			frame := make(map[string]map[string]string)
			So(json.Unmarshal(<-ui, &frame), ShouldBeNil)
			So(frame["activity"]["planner"], ShouldEqual, "done")
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	const payloadBytes = 4096

	ui := make(chan []byte, 1)
	ui <- []byte("existing")
	payload := strings.Repeat("x", payloadBytes)
	b.ReportAllocs()

	for b.Loop() {
		Publish(ui, datura.NewMap("frame", payload))
	}
}

func BenchmarkPublishPriority(b *testing.B) {
	ui := make(chan []byte, 1)
	payload := strings.Repeat("x", 4096)
	b.ReportAllocs()

	for b.Loop() {
		PublishPriority(ui, datura.NewMap("activity", payload))
	}
}
