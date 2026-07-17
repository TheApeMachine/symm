package conditions

import (
	"iter"

	"github.com/theapemachine/symm/tests"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
)

const (
	subjectSymbol = "MATIC/USD"
	peerSymbolA   = "BTC/USD"
	peerSymbolB   = "ETH/USD"
)

func base(
	horizon int,
	ticker tests.Fixture,
	trade tests.Fixture,
	book tests.Fixture,
) *tests.Market {
	return tests.NewMarket().
		Prefix(instrumentfixture.NewFixture(instrumentfixture.SNAPSHOT, 1)).
		Prefix(bookfixture.NewFixture(bookfixture.SNAPSHOT, 1)).
		Feed(ticker).
		Feed(trade).
		Feed(book)
}

func calmTicker(horizon int) tests.Fixture {
	return tickerfixture.NewFixture(tickerfixture.UPDATE, horizon)
}

func calmTrade(horizon int) tests.Fixture {
	return tradefixture.NewFixture(tradefixture.UPDATE, horizon)
}

func calmBook(horizon int) tests.Fixture {
	return bookfixture.NewFixture(bookfixture.UPDATE, horizon)
}

/*
Calm builds a multi-stream market with mild drift and no scenario shaping —
the baseline against which stressed conditions are compared.
*/
func Calm(horizon int) *tests.Market {
	return base(horizon, calmTicker(horizon), calmTrade(horizon), calmBook(horizon))
}

/*
Pump overlays a vertical price and volume surge from frame at onward onto the
calm multi-stream market.
*/
func Pump(horizon int, at int, priceMul float64, volumeMul float64) *tests.Market {
	return base(
		horizon,
		newShaped(tests.Spike(calmTicker(horizon).Frames(), at, priceMul, volumeMul)),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Drawdown ramps price down over the first over frames and holds it — sustained
bleed used for exit/exhaustion style assertions.
*/
func Drawdown(horizon int, depth float64, over int) *tests.Market {
	return base(
		horizon,
		newShaped(tests.Drawdown(calmTicker(horizon).Frames(), depth, over)),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Reversal holds the base trajectory until frame at, then turns price down at
ratePerFrame — momentum flip for exit thesis checks.
*/
func Reversal(horizon int, at int, ratePerFrame float64) *tests.Market {
	return base(
		horizon,
		newShaped(tests.Reversal(calmTicker(horizon).Frames(), at, ratePerFrame)),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Aggression forces buy-heavy trade flow with scaled size from frame at onward.
*/
func Aggression(horizon int, at int, qtyMul float64) *tests.Market {
	return base(
		horizon,
		calmTicker(horizon),
		newShaped(tests.TradeAggression(calmTrade(horizon).Frames(), at, qtyMul)),
		calmBook(horizon),
	)
}

/*
Decay thins resting book qty while keeping the calm tape — mechanical withdrawal.
*/
func Decay(horizon int, at int, depth float64) *tests.Market {
	return base(
		horizon,
		calmTicker(horizon),
		calmTrade(horizon),
		newShaped(tests.BookDecay(twoSidedBooks(horizon), at, depth)),
	)
}

/*
Imbalance loads bids and thins asks from frame at onward — depthflow pressure.
*/
func Imbalance(horizon int, at int, bidMul float64, askMul float64) *tests.Market {
	return base(
		horizon,
		calmTicker(horizon),
		calmTrade(horizon),
		newShaped(tests.BookImbalance(twoSidedBooks(horizon), at, bidMul, askMul)),
	)
}

func twoSidedBooks(horizon int) iter.Seq[tests.Frame] {
	return tests.Repeat(
		bookfixture.NewFixture(bookfixture.SNAPSHOT, 1).Frames(),
		horizon,
	)
}

/*
Lag emits a multi-symbol ticker cohort where followers trail the leader.
*/
func Lag(horizon int, lagFrames int) *tests.Market {
	return base(
		horizon,
		cohortFixture(horizon, tests.CohortLag, lagFrames),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Herd emits a co-moving multi-symbol ticker cohort for peer/breadth signals.
*/
func Herd(horizon int) *tests.Market {
	return base(
		horizon,
		cohortFixture(horizon, tests.CohortHerd, 0),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Noise emits independent multi-symbol ticker paths so correlation can separate
herd from unstructured cohort motion.
*/
func Noise(horizon int) *tests.Market {
	return base(
		horizon,
		cohortFixture(horizon, tests.CohortNoise, 0),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
ThinHerd keeps a co-moving cohort but starves the subject touch quantities so
liquidity scarcity rises relative to peers.
*/
func ThinHerd(horizon int, qtyMul float64) *tests.Market {
	return base(
		horizon,
		newShaped(tests.ThinSubject(
			cohortFixture(horizon, tests.CohortHerd, 0).Frames(),
			subjectSymbol,
			qtyMul,
		)),
		calmTrade(horizon),
		calmBook(horizon),
	)
}

/*
Subject returns the primary MATIC/USD symbol used by single-stream conditions.
*/
func Subject() string {
	return subjectSymbol
}

func cohortFixture(
	horizon int,
	mode tests.CohortMode,
	lagFrames int,
) tests.Fixture {
	var basePayload []byte

	for frame := range calmTicker(1).Frames() {
		basePayload = frame.Payload
		break
	}

	return newShaped(tests.NewCohort(
		basePayload,
		[]string{subjectSymbol, peerSymbolA, peerSymbolB},
		horizon,
		mode,
		lagFrames,
	).Frames())
}

/*
shaped adapts a shaped Frame sequence into the Fixture interface so Market.Feed
can host scenario overlays without a second fixture language.
*/
type shaped struct {
	frames iter.Seq[tests.Frame]
}

/*
newShaped wraps a shaped Frame sequence as a Fixture for Market.Feed.
*/
func newShaped(frames iter.Seq[tests.Frame]) *shaped {
	return &shaped{frames: frames}
}

/*
Generate yields payloads only so Market can treat the overlay like other
fixtures.
*/
func (fixture *shaped) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for frame := range fixture.frames {
			if !yield(frame.Payload) {
				return
			}
		}
	}
}

/*
Frames returns the shaped timeline including channel metadata.
*/
func (fixture *shaped) Frames() iter.Seq[tests.Frame] {
	return fixture.frames
}
