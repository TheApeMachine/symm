package runtime

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewRuntimeSharesCaches(t *testing.T) {
	Convey("Given a pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		defer func() {
			cancel()
			pool.Close()
		}()

		services, err := New(ctx, pool)

		Convey("It should wire independent broker caches", func() {
			So(err, ShouldBeNil)
			So(services.Quotes, ShouldNotBeNil)
			So(services.Stress, ShouldNotBeNil)
			So(services.Rules, ShouldNotBeNil)
			So(services.Audit, ShouldNotBeNil)
		})
	})
}
