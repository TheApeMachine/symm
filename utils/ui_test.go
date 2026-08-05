package utils

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

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
