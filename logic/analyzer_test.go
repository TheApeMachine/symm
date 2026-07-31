package logic

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestAnalyzerOnSignal(t *testing.T) {
	Convey("Given an analyzer thesis gate", t, func() {
		analyzer := &Analyzer{subscribers: &sync.Map{}}
		subscription := types.NewSubscription[any]()
		analyzer.subscribers.Store("thesis", []*types.Subscription[any]{subscription})

		Convey("It should not publish partial signal input", func() {
			thesis := types.NewThesis()
			analyzer.onSignal(thesis)

			select {
			case <-subscription.Channel:
				So("published partial thesis", ShouldEqual, "")
			case <-time.After(20 * time.Millisecond):
			}
		})
	})
}