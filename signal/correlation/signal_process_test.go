package correlation

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestSignalProcessMeasurements(t *testing.T) {
	Convey("Given a correlation signal", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		signal := NewSignal(ctx, pool)

		Convey("It should wire measurement broadcasts", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
			So(signal.Close(), ShouldBeNil)
		})
	})
}
