package strategy

import (
	"bytes"
	"math"
	"slices"
	"testing"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/strategy/impulse"
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
	Convey("Real boundary: same tape -> live Map + live Precursor -> replay Map + replay Precursor -> same fragment-local key at B", t, func() {
		liveMap := impulse.NewMap()
		replayMap := impulse.NewMap()
		livePrecursor := NewPrecursor()
		replayPrecursor := NewPrecursor()

		symbol := "BTC/USD"
		type testTape struct {
			frame   *data.Measurement[float64]
			trade   *data.Measurement[float64]
			signals []*data.Measurement[float64]
		}

		sample := &testTape{
			frame: data.NewMeasurement[float64]("frame", nil),
			trade: data.NewMeasurement[float64]("spot", nil),
		}
		sample.trade.Label = symbol
		sample.trade.Provenance["owner"] = "public"
		sample.trade.Provenance["channel"] = "trade"
		sample.trade.Metadata["venue"] = "true"
		sample.trade.Metadata["volume-unit"] = "base"
		sample.trade.Maturity = 1
		sample.trade.Metrics["qty"] = data.Metric[float64]{Label: "qty", Raw: 1, Exact: decimal.NewFromInt64(1)}
		sample.frame.Peers = append(sample.frame.Peers, sample.trade)

		for index := 0; index < 3; index++ {
			name := string(rune('a' + index))
			measurement := data.NewMeasurement[float64](name, nil)
			measurement.Label = symbol
			measurement.Maturity = 1
			measurement.Provenance["owner"] = name
			measurement.Metrics["value"] = data.Metric[float64]{Label: "value"}
			sample.signals = append(sample.signals, measurement)
			sample.frame.Peers = append(sample.frame.Peers, measurement)
		}

		// Fragments end at ticks 6, 12, 18
		boundaries := map[int64]bool{6: true, 12: true, 18: true}

		for seq := int64(1); seq <= 24; seq++ {
			sample.frame.SeqIdx = seq
			sample.trade.SeqIdx = seq
			for idx, sig := range sample.signals {
				val := math.Sin(float64(seq))
				if idx%2 != 0 {
					val = -val
				}
				sig.SeqIdx = seq
				sig.Metrics["value"] = data.Metric[float64]{Label: "value", Raw: val}
			}

			errLive := liveMap.Step(sample.frame)
			So(errLive, ShouldBeNil)
			// Reverse peers to verify order-independence in replay
			slices.Reverse(sample.frame.Peers)
			errReplay := replayMap.Step(sample.frame)
			So(errReplay, ShouldBeNil)
			slices.Reverse(sample.frame.Peers)

			liveImpulse := liveMap.Markets[symbol].Impulse
			replayImpulse := replayMap.Markets[symbol].Impulse

			So(liveImpulse.Ready, ShouldBeTrue)
			So(replayImpulse.Ready, ShouldBeTrue)

			liveKey := livePrecursor.Encode(&liveImpulse, false)
			replayKey := replayPrecursor.Encode(&replayImpulse, false)

			// Byte-identical fragment-local key across real map + precursor pipelines
			So(bytes.Equal(liveKey, replayKey), ShouldBeTrue)
			if len(liveImpulse.Regions) > 0 {
				So(liveKey, ShouldNotBeNil)
			}

			liveTokens := livePrecursor.Tokens(symbol)
			replayTokens := replayPrecursor.Tokens(symbol)
			So(slices.Equal(liveTokens, replayTokens), ShouldBeTrue)

			// Fragment boundaries reset precursor to ensure fragment-local context
			if boundaries[seq] {
				livePrecursor.Reset(symbol)
				replayPrecursor.Reset(symbol)

				So(livePrecursor.Tokens(symbol), ShouldBeNil)
				So(replayPrecursor.Tokens(symbol), ShouldBeNil)
			}
		}
	})
}
