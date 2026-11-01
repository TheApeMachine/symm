package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPositionLearningShadowTracksBesideOfficial(t *testing.T) {
	Convey("lots evaluate learning beside the active structural stop", t, func() {
		rails := NewAdaptiveRails()
		pos := NewPositionWithRails(
			context.Background(),
			"BTC/USD",
			"ab",
			true,
			LedgerVirtual,
			nil,
			nil,
			nil,
			rails,
		)

		So(pos.structural, ShouldNotBeNil)
		So(pos.learning, ShouldNotBeNil)
		So(pos.ShadowActive, ShouldBeTrue)

		pos.mu.Lock()
		pos.armLossBudgetLocked()
		pos.recordShadowTraceLocked(0.0)
		_, ok0 := pos.learning.Update(0.0)
		pos.recordShadowTraceLocked(1.2)
		_, ok1 := pos.learning.Update(1.2)
		pos.mu.Unlock()

		So(ok0, ShouldBeFalse)
		So(ok1, ShouldBeFalse)
		So(pos.ShadowLive(), ShouldBeTrue)
		So(len(pos.ShadowPnLTrace), ShouldEqual, 2)
	})
}

func TestOfficialSettleKeepsLearningShadowUpdating(t *testing.T) {
	Convey("official structural settle leaves the learning ghost receiving marks", t, func() {
		rails := NewAdaptiveRails()
		desk := &Desk{
			papers:   NewCache(context.Background()),
			reals:    NewCache(context.Background()),
			virtuals: NewCache(context.Background()),
			pending:  &sync.Map{},
			shadows:  &sync.Map{},
			adaptive: rails,
		}

		settled := 0
		pos := &Position{
			ID:           "ghost-1",
			Symbol:       "ETH/USD",
			Book:         "virtual",
			Virtual:      true,
			ShadowActive: true,
			structural:   NewStructuralStop(),
			learning:     NewLearningStop(rails),
			EntryPrice:   dFloat(100),
			Qty:          dFloat(1),
			PnLPct:       dFloat(0.4),
			onSettled: func(p *Position) {
				settled++
				desk.onPositionSettled(p)
			},
		}
		pos.structural.Update(0.0)
		pos.structural.Update(0.5)
		pos.learning.Arm(0)
		pos.learning.Update(0.0)
		pos.learning.Update(0.5)

		now := time.Now().UTC()
		pos.ClosedAt = &now
		pos.OfficialExitAt = &now
		pos.OfficialExitPct = 0.4

		desk.onPositionSettled(pos)

		_, ok := desk.shadows.Load(pos.ID)
		So(ok, ShouldBeTrue)
		So(pos.ShadowLive(), ShouldBeTrue)

		pos.finishShadowLocked("trailing take-profit: test", 0.2)
		desk.onShadowDone(pos)

		_, still := desk.shadows.Load(pos.ID)
		So(still, ShouldBeFalse)
		So(pos.ShadowExitAt, ShouldNotBeNil)
		So(rails.Episodes(), ShouldEqual, 1)
		So(settled, ShouldEqual, 0)
	})
}

func TestPaperLotsEvaluateLearningShadow(t *testing.T) {
	Convey("paper books run structural live and keep a learning shadow", t, func() {
		pos := NewPosition(
			context.Background(),
			"BTC/USD",
			"ab",
			true,
			LedgerPaper,
			nil,
			nil,
			nil,
		)

		So(pos.structural, ShouldNotBeNil)
		So(pos.learning, ShouldNotBeNil)
		So(pos.ShadowActive, ShouldBeTrue)
	})
}
