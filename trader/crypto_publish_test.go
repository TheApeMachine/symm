package trader_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestCryptoApplyPublishesDecisions proves Apply retains the engine tick plus
decisions, lifecycle, and findings frames on the UI hub.
*/
func TestCryptoApplyPublishesDecisions(t *testing.T) {
	Convey("Given a warmed production graph", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Warmup(tests.Idle), ShouldBeNil)

		symbol := market.Symbols[0]
		thesis := wired.Thesis
		thesis.Tick = 7
		thesis.Decisions = []types.Decision{{
			Action: types.ActionNothing,
			Symbol: symbol,
			At:     market.Now(),
			Cause:  "test",
			Reason: "publish proof",
		}}
		thesis.NoteLifecycle(symbol, types.LifecycleObserving, market.Now())
		thesis.Findings = []types.Finding{{
			Symbol:             symbol,
			Component:          "test",
			Condition:          "publish",
			Evidence:           []string{"frame"},
			RequiredValidation: "none",
		}}

		Convey("When Apply finishes a tick with decisions", func() {
			// Let any leftover Actor ticks drain so they do not overwrite the
			// deterministic tick frame this proof publishes next.
			time.Sleep(50 * time.Millisecond)
			thesis.Tick = 7
			wired.Crypto.Apply(thesis)

			Convey("It retains tick, decisions, lifecycle, and findings frames", func() {
				tick := waitCachedContaining(wired, "tick", `"count":7`)
				So(tick, ShouldContainSubstring, `"count":7`)
				So(tick, ShouldContainSubstring, `"completed":true`)
				So(tick, ShouldContainSubstring, `"phase":"complete"`)
				So(waitCached(wired, "decisions"), ShouldContainSubstring, symbol)
				So(waitCached(wired, "decisions"), ShouldContainSubstring, `"nothing"`)
				So(waitCached(wired, "lifecycle"), ShouldContainSubstring, `"observing"`)
				So(waitCached(wired, "findings"), ShouldContainSubstring, `"publish"`)
			})
		})
	})
}

func waitCached(wired *stack.Stack, key string) string {
	return waitCachedContaining(wired, key, key)
}

func waitCachedContaining(wired *stack.Stack, key, needle string) string {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		payload := wired.UIHub.Cached(key)

		if len(payload) > 0 && strings.Contains(string(payload), needle) {
			return string(payload)
		}

		time.Sleep(5 * time.Millisecond)
	}

	return ""
}

/*
BenchmarkCryptoPublish measures one Apply UI publish with a nothing decision.
*/
func BenchmarkCryptoPublish(b *testing.B) {
	market := tests.NewMarket(b.Context(), 1)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		_ = wired.Close()
		market.Close()
	}()

	if err := market.Warmup(tests.Idle); err != nil {
		b.Fatal(err)
	}

	symbol := market.Symbols[0]
	thesis := wired.Thesis
	thesis.Decisions = []types.Decision{{
		Action: types.ActionNothing,
		Symbol: symbol,
		At:     market.Now(),
		Cause:  "bench",
	}}
	thesis.NoteLifecycle(symbol, types.LifecycleObserving, market.Now())

	b.ReportAllocs()

	for b.Loop() {
		wired.Crypto.Apply(thesis)
	}
}
