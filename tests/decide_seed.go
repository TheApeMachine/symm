package tests

import (
	"context"
	"math"
	"path/filepath"
	"testing"
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
balances through the public MockConn). It also aligns the paper CLI wallet so
fills cannot overwrite Available with the shim's default cash pool.
*/
func (session *Session) SeedQuoteCapital(available float64) error {
	if session == nil || session.Paper == nil {
		return errnie.Err(
			errnie.NotFound,
			"tests: session paper unavailable",
			nil,
		)
	}

	if err := session.alignPaperWallet(available); err != nil {
		return err
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

	return session.Paper.Emit("balances", payload)
}

/*
SeedTakerFee stores a percent taker fee on the Price surface for one symbol.
*/
func (session *Session) SeedTakerFee(symbol string, percent float64) error {
	if session == nil || session.Price == nil {
		return errnie.Err(
			errnie.NotFound,
			"tests: session price unavailable",
			nil,
		)
	}

	return session.Price.RememberFee(symbol, kraken.TradeVolumeFee{
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
SeedMatureHolding places an open lot on Balance and marks Thesis lifecycle
managing. Thesis.Holdings stays for Admit-created lots only.
*/
func (session *Session) SeedMatureHolding(
	thesis *types.Thesis,
	symbol string,
	notional float64,
) {
	if thesis == nil || symbol == "" {
		return
	}

	holding := &types.Holding{
		Symbol:     symbol,
		Asset:      baseAsset(symbol),
		Qty:        decimal.NewFromFloat64(notional),
		Mark:       decimal.NewFromFloat64(1),
		EntryPrice: decimal.NewFromFloat64(1),
		Status:     types.OPEN,
		Stoploss:   types.NewStoploss(context.Background()),
	}

	if session != nil && session.Balance != nil {
		session.Balance.Seed(holding)
	}

	thesis.NoteLifecycle(symbol, types.LifecycleManaging, thesis.At)
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

/*
SeedRetreat attaches a toxicity retreating_quantity measurement so Project
feeds RetreatPressure into Stoploss on the next Regulate cut.
*/
func SeedRetreat(thesis *types.Thesis, symbol string, pressure float64) {
	if thesis == nil || symbol == "" || pressure <= 0 {
		return
	}

	normalized := pressure
	thesis.Measurements = append(thesis.Measurements, &types.Measurement{
		Source:     types.SourceToxicity,
		Stream:     types.Toxicity,
		Metric:     types.MetricRetreatingQuantity,
		Subject:    types.SubjectLevel3Touch,
		Symbol:     symbol,
		Side:       types.SideBuy,
		At:         time.Unix(1, 0).UTC(),
		Unit:       types.UnitBaseCurrency,
		Raw:        pressure,
		Normalized: &normalized,
	})
}

/*
PlayOpen plays a market, seeds fees/capital, and installs a bound open lot.
*/
func (session *Session) PlayOpen(
	t testing.TB,
	market *Market,
	symbol string,
	qty, trail float64,
) (*types.Holding, string, error) {
	t.Helper()

	if session == nil || market == nil {
		return nil, "", errnie.Err(errnie.NotFound, "tests: session or market unavailable", nil)
	}

	if _, err := session.Play(market.Frames()); err != nil {
		return nil, "", err
	}

	if err := session.SeedTakerFee(symbol, 0.26); err != nil {
		return nil, "", err
	}

	if err := session.SeedQuoteCapital(10_000); err != nil {
		return nil, "", err
	}

	session.Desk.SetSlots(2, 2)

	return session.SeedOpenLot(t, symbol, qty, trail)
}

/*
SeedOpenLot installs a bound open Holding on Balance and adopts it on Desk.
*/
func (session *Session) SeedOpenLot(
	t testing.TB,
	symbol string,
	qty, trail float64,
) (*types.Holding, string, error) {
	t.Helper()

	if session == nil {
		return nil, "", errnie.Err(errnie.NotFound, "tests: session unavailable", nil)
	}

	statePath := filepath.Join(t.TempDir(), "paper-state.json")
	InstallPaperCLI(t, statePath)

	entry := 1.0

	if ticker, tickerErr := session.Price.Get(symbol); tickerErr == nil &&
		ticker != nil && ticker.Last != nil && ticker.Last.Sign() > 0 {
		entry = ticker.Last.Float64()
	}

	const startingCash = 10_000.0
	cashAfterEnter := startingCash - qty*entry

	if err := session.SeedQuoteCapital(cashAfterEnter); err != nil {
		return nil, "", err
	}

	SetPaperCash(t, statePath, cashAfterEnter)
	SetPaperAsset(t, statePath, baseAsset(symbol), qty)
	SetPaperPrice(t, statePath, symbol, entry)

	at := time.Unix(1, 0).UTC()
	stop := types.NewStoploss(context.Background())
	stop.Bind(entry, trail)
	lot := &types.Holding{
		Symbol:     symbol,
		Asset:      baseAsset(symbol),
		Qty:        decimal.NewFromFloat64(qty),
		EntryPrice: decimal.NewFromFloat64(entry),
		EntryFee:   decimal.NewFromFloat64(qty * entry * 0.0026),
		Mark:       decimal.NewFromFloat64(entry),
		Status:     types.OPEN,
		EntryAt:    &at,
		Stoploss:   stop,
	}
	session.Balance.Seed(lot)

	if session.Desk.OpenPositions() != 1 {
		return nil, "", errnie.Err(
			errnie.Validation,
			"tests: expected one adopted open lot",
			nil,
		)
	}

	return lot, statePath, nil
}

/*
Mark stamps equal bid/last/ask onto Price and fans Desk.Marks.
*/
func (session *Session) Mark(symbol string, mark float64) error {
	return session.MarkQuote(symbol, mark, mark)
}

/*
MarkQuote stamps distinct bid/last onto Price (ask stays above bid) and fans
Desk.Marks for open lots.
*/
func (session *Session) MarkQuote(symbol string, bid, last float64) error {
	if session == nil || session.Price == nil {
		return errnie.Err(errnie.NotFound, "tests: session price unavailable", nil)
	}

	ask := last

	if ask <= bid {
		ask = math.Nextafter(bid, math.Inf(1))
	}

	bidText := decimal.NewFromFloat64(bid).String()
	lastText := decimal.NewFromFloat64(last).String()
	askText := decimal.NewFromFloat64(ask).String()
	session.Price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"` + symbol + `","last":"` + lastText +
			`","bid":"` + bidText + `","ask":"` + askText + `"}]}`,
	))
	session.Desk.Mark(symbol)

	return nil
}

/*
ObserveQuote writes the executable bid onto a seeded lot and calls
Stoploss.ObserveMark so Regulate sees the same mark as the desk fan-out.
*/
func (session *Session) ObserveQuote(lot *types.Holding, bid, last float64) error {
	if lot == nil || lot.Symbol == "" {
		return errnie.Err(errnie.Validation, "tests: lot required", nil)
	}

	if err := session.MarkQuote(lot.Symbol, bid, last); err != nil {
		return err
	}

	lot.Mark = decimal.NewFromFloat64(bid)
	session.Balance.Seed(lot)

	if lot.Stoploss != nil {
		lot.Stoploss.ObserveMark(bid)
	}

	return nil
}

func formatFloat(value float64) string {
	return decimal.NewFromFloat64(value).String()
}

func baseAsset(symbol string) string {
	for index := 0; index < len(symbol); index++ {
		if symbol[index] == '/' {
			return symbol[:index]
		}
	}

	return symbol
}
