package network

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

func TestRecoverable(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When err is nil", func() {
			Convey("Recoverable is false", func() {
				So(Recoverable(nil), ShouldBeFalse)
			})
		})

		Convey("When err is a dial failure", func() {
			dial := errnie.Err(errnie.IO, "[network.websocket] error dialing url", errors.New("dial"))

			Convey("Recoverable is true", func() {
				So(Recoverable(dial), ShouldBeTrue)
			})
		})

		Convey("When err is a reconnect hook failure", func() {
			hook := errnie.Err(errnie.IO, "[network.websocket] reconnect hook failed", errors.New("token"))

			Convey("Recoverable is true", func() {
				So(Recoverable(hook), ShouldBeTrue)
			})
		})

		Convey("When err is a validation bug", func() {
			bug := errnie.Err(errnie.Validation, "[broker] corrupt frame", nil)

			Convey("Recoverable is false", func() {
				So(Recoverable(bug), ShouldBeFalse)
			})
		})
	})
}

func BenchmarkRecoverable(b *testing.B) {
	dial := errnie.Err(errnie.IO, "[network.websocket] error dialing url", errors.New("dial"))

	b.ReportAllocs()

	for b.Loop() {
		_ = Recoverable(dial)
	}
}
