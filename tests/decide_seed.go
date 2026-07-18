package tests

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
	symmmanifold "github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
rawBalanceFrame is a json.Marshaler wrapper so paper.Emit can deliver a
balances snapshot on the same private/paper path Balance.Initialize registers.
*/
type rawBalanceFrame []byte

/*
MarshalJSON returns the raw frame bytes unchanged.
*/
func (frame rawBalanceFrame) MarshalJSON() ([]byte, error) {
	return frame, nil
}

/*
SeedQuoteCapital emits a balances snapshot through the paper transport so
Crypto.Decide sees available quote via BalanceAck (paper mode does not route
balances through the public MockConn).
*/
func (session *Session) SeedQuoteCapital(available float64) error {
	if session == nil || session.paper == nil {
		return errnie.Err(
			errnie.NotFound,
			"tests: session paper unavailable",
			nil,
		)
	}

	amount := formatFloat(available)
	payload := rawBalanceFrame(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","asset_class":"currency",` +
			`"balance":"` + amount + `",` +
			`"available":"` + amount + `",` +
			`"reserved":"0"` +
			`}]}`,
	)

	return session.paper.Emit("balances", payload)
}

/*
SeedTakerFee stores a percent taker fee on the Price surface for one symbol.
*/
func (session *Session) SeedTakerFee(symbol string, percent float64) error {
	if session == nil || session.price == nil {
		return errnie.Err(
			errnie.NotFound,
			"tests: session price unavailable",
			nil,
		)
	}

	return session.price.RememberFee(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(percent),
	})
}

/*
SeedOpportunityForecast attaches a friction-ready next-epoch forecast that
clears utility and, with SeedEarlyCognition, qualifies for reserved overflow.
*/
func SeedOpportunityForecast(
	thesis *types.Thesis,
	symbol string,
	expected, uncertainty float64,
) {
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Source:                   "resonance+causal",
		Symbol:                   symbol,
		At:                       time.Unix(1, 0).UTC(),
		ObservedInterval:         time.Second,
		SourceEpoch:              1,
		HorizonEvents:            1,
		ExpiresEpoch:             2,
		Target:                   "next_l3_epoch_mid_log_return",
		ModelVersion:             "resonance_return_head_v1",
		Ready:                    true,
		Calibrated:               true,
		FrictionReady:            true,
		CalibrationSamples:       8,
		ExpectedReturn:           expected,
		ReferencePrice:           1,
		BuyCapacity:              200,
		SellCapacity:             200,
		ExpectedFees:             0.001,
		ExpectedSpread:           0.001,
		ExpectedImpact:           0,
		ExpectedAdverseSelection: 0,
		Uncertainty:              uncertainty,
		Confidence:               0.8,
		IncrementalMSE:           uncertainty * 0.5,
	})
}

/*
SeedEarlyCognition stores a buy-ready cognition ahead of a mild basin so Lead
is positive for reserved-lane eligibility.
*/
func SeedEarlyCognition(thesis *types.Thesis, symbol string) {
	thesis.Cognition.Store(symbol, types.Cognition{
		Source:     "dmt",
		Symbol:     symbol,
		At:         time.Unix(1, 0).UTC(),
		Winner:     "buy",
		Ready:      true,
		Confidence: 0.7,
		Ambiguous:  false,
		Contrast:   0.4,
	})
	thesis.Manifold.Store(symbol, readyBasin(symbol, 0.2))
}

/*
SeedMatureHolding places an open thesis holding used as an incumbent for
rotate-versus-wait assertions.
*/
func SeedMatureHolding(thesis *types.Thesis, symbol string, notional float64) {
	thesis.Holdings.Store(symbol, types.Holding{
		Symbol: symbol,
		Qty:    decimal.NewFromFloat64(notional),
		Mark:   decimal.NewFromFloat64(1),
		Status: types.OPEN,
	})
}

func readyBasin(symbol string, coherence float64) symmmanifold.State {
	return symmmanifold.State{
		Source:         "manifold",
		Symbol:         symbol,
		At:             time.Unix(1, 0).UTC(),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: 1,
		Spread:         0.001,
		BuyCapacity:    1000,
		SellCapacity:   1000,
		InvalidReason:  symmmanifold.Valid,
		BuyIntensity:   1,
		SellIntensity:  0.5,
		SpectralRadius: 0.1,
		Reading: manifold.Reading{
			PressureGradX: 0.1,
			Divergence:    -0.1,
			CoherenceMag2: coherence,
			GuidanceSpeed: 0.1,
		},
	}
}

func formatFloat(value float64) string {
	return decimal.NewFromFloat64(value).String()
}
