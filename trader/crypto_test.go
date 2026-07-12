package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type cryptoFeedStub struct {
	status types.Status
}

func (feed *cryptoFeedStub) Status() types.Status {
	return feed.status
}

func (feed *cryptoFeedStub) On([]byte) {}

func (feed *cryptoFeedStub) Measure() ([]*types.Measurement, error) {
	return nil, nil
}

func TestNewCrypto(t *testing.T) {
	Convey("Given NewCrypto feed wiring", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer := logic.NewAnalyzer(booter, nil)
		planner := strategy.NewPlanner(booter)

		Convey("When fewer than five feeds are supplied", func() {
			crypto, err := NewCrypto(
				ctx,
				booter,
				nil,
				nil,
				nil,
				nil,
				nil,
				[]types.Feed{
					&cryptoFeedStub{},
					&cryptoFeedStub{},
				},
				nil,
				analyzer,
				planner,
			)

			Convey("Then construction is rejected", func() {
				So(crypto, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When the level3 slot is not a *Level3", func() {
			crypto, err := NewCrypto(
				ctx,
				booter,
				nil,
				nil,
				nil,
				nil,
				nil,
				[]types.Feed{
					&cryptoFeedStub{},
					&cryptoFeedStub{},
					&cryptoFeedStub{},
					&cryptoFeedStub{},
					&cryptoFeedStub{},
				},
				nil,
				analyzer,
				planner,
			)

			Convey("Then construction is rejected", func() {
				So(crypto, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})
	})
}
