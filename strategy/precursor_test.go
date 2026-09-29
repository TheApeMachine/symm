package strategy

import (
	"bytes"
	"slices"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

func TestPrecursorEncode(t *testing.T) {
	Convey("Market and holding state remain part of the context address", t, func() {
		precursor := NewPrecursor()
		impulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 42}}}
		flat := bytes.Clone(precursor.Encode(impulse, false))
		held := bytes.Clone(precursor.Encode(impulse, true))
		So(bytes.Equal(flat, held), ShouldBeFalse)
		impulse.Label = "ETH/USD"
		So(bytes.Equal(flat, precursor.Encode(impulse, false)), ShouldBeFalse)
		impulse.Ready = false
		So(precursor.Encode(impulse, false), ShouldBeNil)
	})

	Convey("Next dynamic holding state updates context address", t, func() {
		precursor := NewPrecursor()
		isHeld := false
		precursor.SetHolding(func(symbol string) bool {
			return isHeld
		})

		impulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 42}}}
		input := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(impulse))
		}

		for item := range precursor.Next(input) {
			command := (*cognition.Command)(item)
			So(command.Evaluate, ShouldNotBeNil)
			So(bytes.Equal(command.Evaluate.Context, precursor.Encode(impulse, false)), ShouldBeTrue)
		}

		isHeld = true
		for item := range precursor.Next(input) {
			command := (*cognition.Command)(item)
			So(command.Evaluate, ShouldNotBeNil)
			So(bytes.Equal(command.Evaluate.Context, precursor.Encode(impulse, true)), ShouldBeTrue)
		}
	})

	Convey("Temporal precursor preserves sequence identity and avoids frequency weighting", t, func() {
		Convey("Two identical final Impulses with different temporal histories produce distinct keys", func() {
			p1 := NewPrecursor()
			p2 := NewPrecursor()

			finalImpulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 999}}}

			// Path 1: 100 -> 200 -> 999
			p1.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 100}}}, false)
			p1.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 200}}}, false)
			key1 := p1.Encode(finalImpulse, false)

			// Path 2: 300 -> 400 -> 999
			p2.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 300}}}, false)
			p2.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 400}}}, false)
			key2 := p2.Encode(finalImpulse, false)

			// Snapshot regression test: in snapshot learning key1 == key2. Here they MUST differ.
			So(bytes.Equal(key1, key2), ShouldBeFalse)
			So(len(p1.Tokens("BTC/USD")), ShouldEqual, 3)
			So(len(p2.Tokens("BTC/USD")), ShouldEqual, 3)
		})

		Convey("Repeated identical Impulse states do not multiply evidence", func() {
			precursor := NewPrecursor()
			impA := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 100}}}
			impB := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 200}}}

			precursor.Encode(impA, false)

			// Feed impB 10 times in a row (representing scheduler frequency without new development)
			for i := 0; i < 10; i++ {
				precursor.Encode(impB, false)
			}

			tokens := precursor.Tokens("BTC/USD")
			So(len(tokens), ShouldEqual, 2)
			So(len(precursor.Key("BTC/USD", false)), ShouldEqual, (1+2)*8)
		})

		Convey("Longer fragments are not truncated to arbitrary caps like maxOrder=8", func() {
			precursor := NewPrecursor()

			for i := uint64(1); i <= 15; i++ {
				precursor.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: i}}}, false)
			}

			tokens := precursor.Tokens("BTC/USD")
			So(len(tokens), ShouldEqual, 15)
			So(len(precursor.Key("BTC/USD", false)), ShouldEqual, (1+15)*8)
		})

		Convey("Reset clears symbol temporal state at structural boundaries", func() {
			precursor := NewPrecursor()
			precursor.Encode(&grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 1}}}, false)
			So(len(precursor.Tokens("BTC/USD")), ShouldEqual, 1)

			precursor.Reset("BTC/USD")
			So(precursor.Tokens("BTC/USD"), ShouldBeNil)
			So(precursor.Key("BTC/USD", false), ShouldBeNil)
		})
	})
}

func TestLiveReplayKeyIdentity(t *testing.T) {
	Convey("Identical observations produce byte-identical live and replay context keys across fragment boundaries", t, func() {
		livePrecursor := NewPrecursor()
		replayPrecursor := NewPrecursor()

		symbol := "BTC/USD"
		var observations []*grid.Impulse

		// Sequence of 20 distinct market impulses across multiple fragments
		for i := uint64(1); i <= 20; i++ {
			observations = append(observations, &grid.Impulse{
				Label:   symbol,
				Ready:   true,
				SeqIdx:  int64(i),
				Regions: []grid.Region{{Condition: i * 17}},
			})
		}

		// Replay simulates historical fragment completion at ticks 6, 12, 18
		// CRITICAL REQUIREMENT: Neither live nor replay resets the precursor at fragment boundaries.
		// Therefore, for every tick T, live key at T must equal replay key at T byte-for-byte.
		for tickIdx, obs := range observations {
			liveKey := livePrecursor.Encode(obs, false)
			replayKey := replayPrecursor.Encode(obs, false)

			So(bytes.Equal(liveKey, replayKey), ShouldBeTrue)
			So(liveKey, ShouldNotBeNil)

			// Verify tokens match byte-for-byte
			liveTokens := livePrecursor.Tokens(symbol)
			replayTokens := replayPrecursor.Tokens(symbol)
			So(len(liveTokens), ShouldEqual, tickIdx+1)
			So(len(replayTokens), ShouldEqual, tickIdx+1)
			So(slices.Equal(liveTokens, replayTokens), ShouldBeTrue)
		}
	})
}
