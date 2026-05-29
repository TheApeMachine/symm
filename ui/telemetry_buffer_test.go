package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTelemetryBufferPush(t *testing.T) {
	Convey("Given a full telemetry buffer", t, func() {
		buffer := NewTelemetryBuffer(2)

		buffer.Push("oldest")
		buffer.Push("middle")
		buffer.Push("newest")

		Convey("It should drop the oldest frame and keep recent telemetry", func() {
			So(buffer.Dropped(), ShouldEqual, 1)
			So(<-buffer.queue, ShouldEqual, "middle")
			So(<-buffer.queue, ShouldEqual, "newest")
		})
	})
}

func TestTelemetryBufferRun(t *testing.T) {
	Convey("Given a telemetry consumer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		buffer := NewTelemetryBuffer(2)
		seen := make(chan any, 1)

		go buffer.Run(ctx, func(payload any) {
			seen <- payload
		})

		buffer.Push("frame")

		Convey("It should deliver queued frames asynchronously", func() {
			select {
			case payload := <-seen:
				So(payload, ShouldEqual, "frame")
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for telemetry frame")
			}
		})
	})
}

func BenchmarkTelemetryBufferPush(b *testing.B) {
	buffer := NewTelemetryBuffer(512)

	b.ReportAllocs()

	for b.Loop() {
		buffer.Push("frame")
	}
}
