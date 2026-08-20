package utils

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPublish(t *testing.T) {
	Convey("Given a lock-free UI transport", t, func() {
		ui := transport.NewMapReduce[[]byte]([]string{"test"}, nil, nil)

		Convey("It should enqueue a marshaled frame without blocking", func() {
			Publish(ui, datura.NewMap("ticker", "new"))

			frame, ok := ui.Pop("test")
			So(ok, ShouldBeTrue)
			So(string(frame), ShouldContainSubstring, `"ticker":"new"`)
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	const payloadBytes = 4096

	ui := transport.NewMapReduce[[]byte]([]string{"bench"}, nil, nil)
	payload := strings.Repeat("x", payloadBytes)
	b.ReportAllocs()

	for b.Loop() {
		Publish(ui, datura.NewMap("frame", payload))
	}
}
