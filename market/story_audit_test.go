package market

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestStoryPlaybookWalkAudit(t *testing.T) {
	convey.Convey("Given a story with an audit file", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")

		viper.Set("trading.audit.file", auditPath)
		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		trading.MarkDeskReady()
		defer viper.Set("trading.audit.file", "")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.quote_currency", "")
		defer trading.ResetDeskReady()

		story, storyErr := NewStory(ctx, pool, focus.NewSet())
		convey.So(storyErr, convey.ShouldBeNil)

		done := make(chan struct{})

		go func() {
			_ = story.Tick()
			close(done)
		}()

		measurements := story.broadcasts["measurements"]
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryVolumeStarvation,
			SNR:      1.0,
			Last:     50_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.011867,
			Last:     50_000,
		}})
		measurements.Send(&qpool.QValue[any]{Value: perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Category: perspectives.CategoryLiquidityVacuum,
			SNR:      1.0,
			Last:     50_000,
		}})

		time.Sleep(100 * time.Millisecond)

		cancel()
		<-done
		convey.So(story.Close(), convey.ShouldBeNil)

		raw, readErr := os.ReadFile(auditPath)

		convey.Convey("It should write a playbook walk audit line", func() {
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, `"audit_event":"playbook_walk"`)
			convey.So(string(raw), convey.ShouldContainSubstring, `"symbol":"BTC/EUR"`)
			convey.So(string(raw), convey.ShouldContainSubstring, `"steps"`)
		})
	})
}

func TestPlaybookWalkDedup(t *testing.T) {
	convey.Convey("Given repeated identical walk outcomes", t, func() {
		dedup := newPlaybookWalkDedup()
		dedup.cooldown = time.Minute
		audit := &perspectives.WalkAudit{VerdictDepth: 1}
		limit := perspectives.ActionLimit
		audit.Verdict = &limit

		first := dedup.shouldLog("BTC/EUR", audit, "")
		second := dedup.shouldLog("BTC/EUR", audit, "")

		convey.Convey("It should deduplicate within the cooldown window", func() {
			convey.So(first, convey.ShouldBeTrue)
			convey.So(second, convey.ShouldBeFalse)
		})
	})
}

func TestPlaybookWalkFrame(t *testing.T) {
	convey.Convey("Given a walk audit trace", t, func() {
		limit := perspectives.ActionLimit
		audit := &perspectives.WalkAudit{
			Verdict:      &limit,
			VerdictDepth: 0,
			Steps: []perspectives.BranchStep{{
				Depth: 0,
				Pass:  true,
			}},
		}

		frame, err := playbookWalkFrame(
			perspectives.Measurement{Symbol: "BTC/EUR", Last: 50_000},
			audit,
			nil,
			"",
		)

		convey.Convey("It should encode the verdict and steps", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(frame["audit_event"], convey.ShouldEqual, "playbook_walk")
			convey.So(frame["verdict"], convey.ShouldEqual, perspectives.ActionLimit)
		})
	})
}
