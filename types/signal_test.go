package types

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubSignal struct {
	name string
	err  error
	typ  SourceType
}

func (s *stubSignal) Name() string     { return s.name }
func (s *stubSignal) Error() error     { return s.err }
func (s *stubSignal) Type() SourceType { return s.typ }
func (s *stubSignal) Close() error     { return s.err }

var (
	_ Signal       = (*stubSignal)(nil)
	_ CohortSignal = (*stubSignal)(nil)
)

func TestName(t *testing.T) {
	Convey("Setup", t, func() {
		sig := &stubSignal{name: "cvd", typ: SourceCVD}

		Convey("When Name is called", func() {
			Convey("it returns the signal name", func() {
				So(sig.Name(), ShouldEqual, "cvd")
			})
		})
	})
}

func TestError(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Error is called with no fault", func() {
			sig := &stubSignal{name: "cvd", typ: SourceCVD}

			Convey("it returns nil", func() {
				So(sig.Error(), ShouldBeNil)
			})
		})

		Convey("When Error is called with a fault", func() {
			sig := &stubSignal{
				name: "cvd",
				typ:  SourceCVD,
				err:  errors.New("[signal] closed"),
			}

			Convey("it returns the exact fault", func() {
				So(sig.Error(), ShouldNotBeNil)
				So(sig.Error().Error(), ShouldEqual, "[signal] closed")
			})
		})
	})
}

func TestType(t *testing.T) {
	Convey("Setup", t, func() {
		sig := &stubSignal{name: "hawkes", typ: SourceHawkes}

		Convey("When Type is called", func() {
			Convey("it returns the source type", func() {
				So(sig.Type(), ShouldEqual, SourceHawkes)
				So(string(sig.Type()), ShouldEqual, "hawkes")
			})
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Close is called with no fault", func() {
			sig := &stubSignal{name: "cvd", typ: SourceCVD}

			Convey("it returns nil", func() {
				So(sig.Close(), ShouldBeNil)
			})
		})

		Convey("When Close is called with a fault", func() {
			sig := &stubSignal{
				name: "cvd",
				typ:  SourceCVD,
				err:  errors.New("[signal] already closed"),
			}

			Convey("it returns the exact fault", func() {
				err := sig.Close()
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "[signal] already closed")
			})
		})
	})
}
