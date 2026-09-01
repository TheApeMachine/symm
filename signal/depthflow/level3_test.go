package depthflow

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
level3Order builds one kraken.Level3Order fixture with a non-nil price/qty.
Use level3OrderNilPrice/level3OrderNilQty for the malformed variants.
*/
var level3OrderSeq int

/*
level3Order builds one order fixture with a UNIQUE identity. The resident book
is keyed by OrderID, so fixtures sharing one id would alias to a single order
and silently overwrite each other.
*/
func level3Order(event string, price float64, qty float64, at time.Time) kraken.Level3Order {
	level3OrderSeq++

	return kraken.Level3Order{
		Event:      event,
		OrderID:    fmt.Sprintf("order-%d", level3OrderSeq),
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

/*
level3OrderID builds an order fixture with an explicit identity, so a delete
can name the very order an earlier step added. The resident book removes an
order's OWN resting notional, so a delete naming an unknown id correctly
removes nothing.
*/
func level3OrderID(id string, event string, price float64, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      event,
		OrderID:    id,
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func level3OrderNilPrice(event string, qty float64, at time.Time) kraken.Level3Order {
	level3OrderSeq++

	return kraken.Level3Order{
		Event:      event,
		OrderID:    fmt.Sprintf("order-%d", level3OrderSeq),
		LimitPrice: nil,
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func level3OrderNilQty(event string, price float64, at time.Time) kraken.Level3Order {
	level3OrderSeq++

	return kraken.Level3Order{
		Event:      event,
		OrderID:    fmt.Sprintf("order-%d", level3OrderSeq),
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   nil,
		Timestamp:  at,
	}
}

/*
level3Message builds one kraken.Level3Data fixture from bid/ask order lists.
*/
func level3Message(symbol string, at time.Time, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      bids,
		Asks:      asks,
	}
}

/*
seededLevel3 returns an entity whose book has been established by an empty
snapshot. Kraken opens every L3 subscription with a snapshot frame, and the
resident book refuses to report depth before one arrives -- an increment alone
describes a book the process never saw.
*/
func seededLevel3() *Level3 {
	entity := NewLevel3()
	entity.Step(kraken.Level3Data{
		Symbol:    "BTC/USD",
		Type:      "snapshot",
		Timestamp: baseTime,
	})

	for _, symbol := range []string{"ETH/USD", "SOL/USD", "XRP/USD", "ADA/USD"} {
		entity.Step(kraken.Level3Data{
			Symbol:    symbol,
			Type:      "snapshot",
			Timestamp: baseTime,
		})
	}

	return entity
}

func metric(measurement *data.Measurement[float64], name string) float64 {
	if measurement == nil {
		return 0
	}

	return measurement.Metrics[name].Raw
}

func hasMetric(measurement *data.Measurement[float64], name string) bool {
	if measurement == nil {
		return false
	}

	_, found := measurement.Metrics[name]

	return found
}

var (
	baseTime  = time.Unix(1_700_000_000, 0)
	nextTime  = time.Unix(1_700_000_001, 0)
	thirdTime = time.Unix(1_700_000_002, 0)
)

/*
level3Step is one message applied to an entity, with the assertions to run
against its resulting Measurement.
*/
type level3Step struct {
	name    string
	message kraken.Level3Data
	assert  func(measurement *data.Measurement[float64])
}

/*
level3Case is a full scenario: a fresh Level3 entity driven through its steps
in order, so later steps can assert on accumulated/estimator state.
*/
type level3Case struct {
	name  string
	steps []level3Step
}

/*
TestLevel3Step_Valid drives Step with well-formed Level3 messages. Each table
case gets its own fresh entity and Convey scope; multi-step cases assert on
prior steps too, since they exercise accumulation and estimator warmup.
*/
func TestLevel3Step_Valid(t *testing.T) {
	validCases := []level3Case{
		{
			name: "first observation seeds book notional and touch, no warmup-only metrics",
			steps: []level3Step{
				{
					name: "add one bid and one ask",
					message: level3Message("BTC/USD", baseTime,
						[]kraken.Level3Order{level3Order("add", 99, 2, baseTime)},
						[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement, ShouldNotBeNil)
						So(measurement.Err, ShouldBeNil)

						So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 198.0, 1e-9)
						So(metric(measurement, "book_notional:ask"), ShouldAlmostEqual, 202.0, 1e-9)
						So(metric(measurement, "book_notional"), ShouldAlmostEqual, 400.0, 1e-9)
						So(metric(measurement, "book_imbalance"), ShouldAlmostEqual, -0.01, 1e-9)

						// First observation: support is one sample, so maturity is
						// zero and the rate metrics (need a prior interval) are absent.
						So(measurement.Maturity, ShouldEqual, 0.0)
						So(hasMetric(measurement, "net_displayed_flow_rate:bid"), ShouldBeFalse)
						So(hasMetric(measurement, "book_turnover_rate"), ShouldBeFalse)
						So(hasMetric(measurement, "book_imbalance_zscore"), ShouldBeFalse)
					},
				},
			},
		},
		{
			name: "an add on a one-sided second message accumulates into the running book notional",
			steps: []level3Step{
				{
					name: "seed",
					message: level3Message("BTC/USD", baseTime,
						[]kraken.Level3Order{level3Order("add", 99, 2, baseTime)},
						[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement.Err, ShouldBeNil)
					},
				},
				{
					name: "add more bid depth, no ask side in this message",
					message: level3Message("BTC/USD", nextTime,
						[]kraken.Level3Order{level3Order("add", 99, 4, nextTime)},
						nil,
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement, ShouldNotBeNil)
						So(measurement.Err, ShouldBeNil)

						// Running total accumulates: 198 (seed) + 396 (this add) = 594.
						// A one-sided message must not stall accumulation.
						So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 594.0, 1e-9)

						// This message's own signed delta, not a diff of two totals.
						So(metric(measurement, "net_displayed_flow:bid"), ShouldAlmostEqual, 396.0, 1e-9)
						So(metric(measurement, "added_notional:bid"), ShouldAlmostEqual, 396.0, 1e-9)
						So(metric(measurement, "removed_notional:bid"), ShouldAlmostEqual, 0.0, 1e-9)

						So(measurement.Maturity, ShouldEqual, 0.5)
					},
				},
			},
		},
		{
			name: "a delete contributes a negative delta",
			steps: []level3Step{
				{
					name: "seed both sides",
					message: level3Message("ETH/USD", baseTime,
						[]kraken.Level3Order{level3OrderID("eth-bid", "add", 50, 10, baseTime)},
						[]kraken.Level3Order{level3OrderID("eth-ask", "add", 51, 10, baseTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement.Err, ShouldBeNil)
					},
				},
				{
					name: "delete the bid, no ask side in this message",
					message: level3Message("ETH/USD", nextTime,
						[]kraken.Level3Order{level3OrderID("eth-bid", "delete", 50, 10, nextTime)},
						nil,
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement, ShouldNotBeNil)

						// 500 (seed) - 500 (deleted) = 0.
						So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 0.0, 1e-9)
						So(metric(measurement, "net_displayed_flow:bid"), ShouldAlmostEqual, -500.0, 1e-9)
						So(metric(measurement, "removed_notional:bid"), ShouldAlmostEqual, 500.0, 1e-9)
						So(metric(measurement, "added_notional:bid"), ShouldAlmostEqual, 0.0, 1e-9)
					},
				},
			},
		},
		{
			name: "touch price reflects the best add seen this message, not a higher-priced delete",
			steps: []level3Step{
				{
					name: "two bids, one ask",
					message: level3Message("SOL/USD", baseTime,
						[]kraken.Level3Order{
							level3OrderID("sol-bid-20", "add", 20, 1, baseTime),
							level3Order("add", 19, 1, baseTime),
						},
						[]kraken.Level3Order{level3Order("add", 21, 1, baseTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement.Err, ShouldBeNil)
						So(metric(measurement, "book_notional"), ShouldBeGreaterThan, 0)
					},
				},
				{
					name: "a higher-priced delete must not win over a lower-priced add",
					message: level3Message("SOL/USD", nextTime,
						[]kraken.Level3Order{
							level3OrderID("sol-bid-20", "delete", 20, 1, nextTime),
							level3Order("add", 18, 1, nextTime),
						},
						[]kraken.Level3Order{level3Order("add", 22, 1, nextTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement, ShouldNotBeNil)
						So(measurement.Err, ShouldBeNil)

						// +18 (new order) - 20 (the resident order this delete
						// names) = -2. The removal is the DELETED ORDER's own
						// resting notional, not whatever the delete message
						// happened to quote.
						So(metric(measurement, "net_displayed_flow:bid"), ShouldAlmostEqual, -2.0, 1e-9)
					},
				},
			},
		},
		{
			name: "a third observation matures the turnover estimator chain",
			steps: []level3Step{
				{
					name: "seed",
					message: level3Message("XRP/USD", baseTime,
						[]kraken.Level3Order{level3Order("add", 1, 100, baseTime)},
						[]kraken.Level3Order{level3Order("add", 1.01, 100, baseTime)},
					),
				},
				{
					name: "second",
					message: level3Message("XRP/USD", nextTime,
						[]kraken.Level3Order{level3Order("add", 1, 50, nextTime)},
						[]kraken.Level3Order{level3Order("add", 1.01, 50, nextTime)},
					),
				},
				{
					name: "third",
					message: level3Message("XRP/USD", thirdTime,
						[]kraken.Level3Order{level3Order("add", 1, 25, thirdTime)},
						[]kraken.Level3Order{level3Order("add", 1.01, 25, thirdTime)},
					),
					assert: func(measurement *data.Measurement[float64]) {
						So(measurement, ShouldNotBeNil)
						So(measurement.Err, ShouldBeNil)
						So(hasMetric(measurement, "turnover_zscore"), ShouldBeTrue)
						So(hasMetric(measurement, "turnover_velocity"), ShouldBeTrue)
					},
				},
			},
		},
	}

	Convey("Given well-formed Level3 messages", t, func() {
		for _, testCase := range validCases {
			Convey(testCase.name, func() {
				entity := seededLevel3()

				for _, step := range testCase.steps {
					measurement := entity.Step(step.message)

					if step.assert != nil {
						Convey(step.name, func() {
							step.assert(measurement)
						})
					}
				}
			})
		}
	})
}

/*
TestLevel3Step_Adversarial drives Step with malformed or degenerate Level3
messages: missing price/qty fields, empty sides, and a crossed touch. Step
must never panic, and must fail closed (nil measurement, or a measurement
carrying a non-nil Err) exactly where the book genuinely cannot be trusted —
never merely because one side was quiet this message.
*/
func TestLevel3Step_Adversarial(t *testing.T) {
	adversarialCases := []struct {
		name    string
		message kraken.Level3Data
		assert  func(measurement *data.Measurement[float64])
	}{
		{
			name: "empty bids side is a normal one-sided update, not a rejection",
			message: level3Message("BTC/USD", baseTime,
				nil,
				[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)
				So(metric(measurement, "book_notional:ask"), ShouldAlmostEqual, 202.0, 1e-9)
			},
		},
		{
			name: "empty asks side is a normal one-sided update, not a rejection",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{level3Order("add", 99, 2, baseTime)},
				nil,
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)
				So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 198.0, 1e-9)
			},
		},
		{
			name:    "both sides empty yields no measurement",
			message: level3Message("BTC/USD", baseTime, nil, nil),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldBeNil)
			},
		},
		{
			// A two-sided touch price does not imply a two-sided touch
			// notional: an add/change order can quote a level at zero
			// quantity, leaving touch_notional at exactly zero. Dividing by
			// it set Frame.Err, and Projector.Project then discarded the
			// whole measurement -- including the notional accumulation that
			// had nothing to do with the touch.
			name: "zero-quantity touch on both sides leaves touch_imbalance absent, not an error",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{level3Order("add", 99, 0, baseTime)},
				[]kraken.Level3Order{level3Order("add", 101, 0, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)

				So(hasMetric(measurement, "touch_imbalance"), ShouldBeFalse)
				So(hasMetric(measurement, "imbalance_resolution_gap"), ShouldBeFalse)
			},
		},
		{
			name: "crossed touch (bid above ask) fails closed with a pipeline error",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{level3Order("add", 101, 1, baseTime)},
				[]kraken.Level3Order{level3Order("add", 99, 1, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldNotBeNil)
			},
		},
		{
			name: "nil LimitPrice orders are skipped rather than panicking",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{
					level3OrderNilPrice("add", 2, baseTime),
					level3Order("add", 99, 2, baseTime),
				},
				[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)

				// Only the valid order (99*2=198) counts; the nil-price order
				// is skipped, not treated as a zero-price contribution.
				So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 198.0, 1e-9)
			},
		},
		{
			name: "nil OrderQty orders are skipped rather than panicking",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{
					level3OrderNilQty("add", 98, baseTime),
					level3Order("add", 99, 2, baseTime),
				},
				[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)
				So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 198.0, 1e-9)
			},
		},
		{
			name: "a lone delete of orders the book never held removes nothing",
			message: level3Message("BTC/USD", baseTime,
				[]kraken.Level3Order{level3Order("delete", 99, 2, baseTime)},
				[]kraken.Level3Order{level3Order("delete", 101, 2, baseTime)},
			),
			assert: func(measurement *data.Measurement[float64]) {
				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)

				// The book removes an order's OWN resting notional, so a
				// delete naming an order it never held removes nothing. The
				// old signed-delta sum instead subtracted the quoted notional
				// and drove book depth negative out of thin air.
				So(hasMetric(measurement, "touch_imbalance"), ShouldBeFalse)
				So(metric(measurement, "net_displayed_flow:bid"), ShouldAlmostEqual, 0.0, 1e-9)
				So(metric(measurement, "book_notional:bid"), ShouldAlmostEqual, 0.0, 1e-9)
			},
		},
	}

	Convey("Given malformed or degenerate Level3 messages", t, func() {
		for _, testCase := range adversarialCases {
			Convey(testCase.name, func() {
				// The panic check runs the message twice in a row on its own
				// entity: a single isolated call can't surface a bug that only
				// appears once committed state exists from a prior step (a
				// real venue stream is a sequence, never one message in a
				// vacuum), and this entity is kept separate from the one below
				// so its extra call never perturbs the assertion.
				panicEntity := seededLevel3()

				So(func() {
					panicEntity.Step(testCase.message)
					panicEntity.Step(testCase.message)
				}, ShouldNotPanic)

				entity := seededLevel3()
				measurement := entity.Step(testCase.message)
				testCase.assert(measurement)
			})
		}
	})

	Convey("A rejected message never corrupts committed state for the messages around it", t, func() {
		entity := seededLevel3()

		good := entity.Step(level3Message("BTC/USD", baseTime,
			[]kraken.Level3Order{level3Order("add", 99, 2, baseTime)},
			[]kraken.Level3Order{level3Order("add", 101, 2, baseTime)},
		))
		So(good.Err, ShouldBeNil)
		So(metric(good, "book_notional:bid"), ShouldAlmostEqual, 198.0, 1e-9)

		crossed := entity.Step(level3Message("BTC/USD", nextTime,
			[]kraken.Level3Order{level3Order("add", 200, 1, nextTime)},
			[]kraken.Level3Order{level3Order("add", 50, 1, nextTime)},
		))
		So(crossed.Err, ShouldNotBeNil)

		next := entity.Step(level3Message("BTC/USD", thirdTime,
			[]kraken.Level3Order{level3Order("add", 99, 1, thirdTime)},
			nil,
		))
		So(next.Err, ShouldBeNil)

		// 198 (seed) + 99 (this add) = 297. If the crossed/rejected message
		// had leaked its delta into committed state despite failing, this
		// would be 198 + 200 + 99 = 497 instead.
		So(metric(next, "book_notional:bid"), ShouldAlmostEqual, 297.0, 1e-9)
	})
}

/*
BenchmarkLevel3Step measures the steady-state cost and allocation count of one
Step against a realistic ten-level message, once warm.
*/
func BenchmarkLevel3Step(b *testing.B) {
	entity := seededLevel3()

	bids := make([]kraken.Level3Order, 0, 10)
	asks := make([]kraken.Level3Order, 0, 10)

	for level := range 10 {
		bids = append(bids, level3Order("add", 99-float64(level), 2, baseTime))
		asks = append(asks, level3Order("add", 101+float64(level), 2, baseTime))
	}

	message := level3Message("BTC/USD", baseTime, bids, asks)

	// Warm the estimator chains so the steady-state path (not first-observation
	// warmup) is what gets measured.
	entity.Step(message)
	entity.Step(level3Message("BTC/USD", nextTime, bids, asks))

	b.ReportAllocs()

	for b.Loop() {
		entity.Step(message)
	}
}

/*
BenchmarkLevel3Step_ManyOrders measures Step's cost scaling with message size,
at a depth well beyond what one venue update ordinarily carries.
*/
func BenchmarkLevel3Step_ManyOrders(b *testing.B) {
	entity := seededLevel3()

	const depth = 500

	bids := make([]kraken.Level3Order, 0, depth)
	asks := make([]kraken.Level3Order, 0, depth)

	for level := range depth {
		bids = append(bids, level3Order("add", 100-float64(level)*0.01, 1, baseTime))
		asks = append(asks, level3Order("add", 100.5+float64(level)*0.01, 1, baseTime))
	}

	message := level3Message("BTC/USD", baseTime, bids, asks)

	entity.Step(message)

	b.ReportAllocs()

	for b.Loop() {
		entity.Step(message)
	}
}

/*
TestLevel3Step_SameTimestampIncrements pins the production failure. Kraken
batches L3 updates within a single clock tick, so consecutive increments
routinely carry an identical timestamp. Every velocity chain then divided its
delta by an elapsed interval of exactly zero and failed the whole frame -- the
"quotient denominator must be non-zero" flood.

Every other fixture in this package uses distinct timestamps, which is exactly
why the suite stayed green while production did not.
*/
func TestLevel3Step_SameTimestampIncrements(t *testing.T) {
	Convey("Given increments that all share one event timestamp", t, func() {
		at := bookAt()
		entity := NewLevel3()

		entity.Step(snapshot("BTC/USD",
			[]kraken.Level3Order{level3OrderID("a", "add", 99, 10, at)},
			[]kraken.Level3Order{level3OrderID("b", "add", 101, 10, at)},
		))

		Convey("No frame fails on a zero elapsed interval", func() {
			for _, id := range []string{"c", "d", "e", "f"} {
				measurement := entity.Step(increment("BTC/USD",
					[]kraken.Level3Order{level3OrderID(id, "add", 99, 1, at)},
					nil,
				))

				So(measurement, ShouldNotBeNil)
				So(measurement.Err, ShouldBeNil)
			}
		})
	})
}
