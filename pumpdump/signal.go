package pumpdump

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	*engine.SignalBase
	track *TrackStore
}

var _ engine.Signal = (*PumpDump)(nil)

var _ engine.FeedbackReceiver = (*PumpDump)(nil)

var _ engine.LiveScoreReader = (*PumpDump)(nil)

/*
LiveScore returns the current pump gauge reading from track state.
*/
func (pumpdump *PumpDump) LiveScore() float64 {
	return pumpdump.track.PeakLiveConfidence()
}

func (pumpdump *PumpDump) PeakReading() engine.LiveReading {
	symbol, score := pumpdump.track.PeakSymbolScore()

	return engine.LiveReading{
		Symbol: symbol,
		Score:  score,
	}
}

/*
NewPumpDump wires live Kraken websocket observers into the engine signal.
*/
func NewPumpDump(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*PumpDump, error) {
	base, err := engine.NewSignalBase(
		ctx,
		"pumpdump",
		book,
		tradesObserver,
		tickerObserver,
		pairs,
		symbols,
		watch,
	)

	if err != nil {
		return nil, err
	}

	pumpdump := &PumpDump{
		SignalBase: base,
		track:      NewTrackStore(),
	}

	return pumpdump, errnie.Require(map[string]any{
		"base":  base,
		"track": pumpdump.track,
	})
}

/*
ApplyFeedback nudges precursor calibration from settled prediction error.
*/
func (pumpdump *PumpDump) ApplyFeedback(feedback engine.PredictionFeedback) {
	if feedback.Source != pumpdump.Source() {
		return
	}

	pumpdump.track.ApplyPredictionFeedback(feedback)
}

/*
Scan samples microstructure for the current scheduler tick.
*/
func (pumpdump *PumpDump) Scan(now time.Time) error {
	pumpdump.track.BeginScan()
	pumpdump.refreshTracks(now)
	pumpdump.track.RollBuckets(now)

	return pumpdump.ScanSymbols(now, func(
		symbol string, snapshot engine.Snapshot,
	) (engine.Measurement, bool, error) {
		return pumpdump.evaluate(symbol, snapshot, now)
	})
}

func (pumpdump *PumpDump) refreshTracks(now time.Time) {
	for _, symbol := range pumpdump.Symbols() {
		snapshot := pumpdump.Ingest().Read(symbol)

		if snapshot.LastOK && snapshot.VolumeOK {
			pumpdump.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
		}

		if snapshot.BatchOK {
			pumpdump.track.AddVolume(symbol, snapshot.BatchVolume, now)
		}

		if snapshot.SpreadOK {
			pumpdump.track.RecordSpread(symbol, snapshot.SpreadBPS)
		}
	}

	_ = now
}

func (pumpdump *PumpDump) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	now time.Time,
) (engine.Measurement, bool, error) {
	rawConfidence, reason := pumpdump.score(symbol, snapshot)

	if rawConfidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	if reason == "" {
		reason = "precursor"
	}

	confidence, expectedReturn, runway, reason := pumpdump.track.FinalizeMeasurement(
		symbol, rawConfidence, now, reason,
	)

	if confidence <= 0 || expectedReturn <= 0 || runway <= 0 {
		return engine.Measurement{}, false, nil
	}

	return engine.Measurement{
		Type:           engine.Pump,
		Regime:         "pump",
		Reason:         reason,
		Confidence:     confidence,
		ExpectedReturn: expectedReturn,
		Runway:         runway,
	}, true, nil
}

func (pumpdump *PumpDump) score(symbol string, snapshot engine.Snapshot) (float64, string) {
	if !pumpdump.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	volumeRatio, volumeSpike := pumpdump.track.VolumeSpike(symbol)

	if !snapshot.ImbalanceOK || !snapshot.PressureOK {
		return 0, ""
	}

	micro := precursorScore(snapshot.Imbalance, snapshot.BuyPressure)

	if micro <= 0 || volumeRatio <= 0 {
		return 0, ""
	}

	calibration := pumpdump.track.CalibrationScale(symbol)

	if calibration <= 0 {
		return 0, ""
	}

	confidence := volumeRatio * micro * calibration
	reason := "precursor"

	if !volumeSpike {
		return confidence, reason
	}

	if !pumpdump.track.PriceFlat(symbol) {
		return confidence, reason
	}

	if !snapshot.SpreadOK || !pumpdump.track.SpreadTight(symbol, snapshot.SpreadBPS) {
		return confidence, reason
	}

	return confidence, "actual_pump"
}

/*
precursorScore requires bid-side book pressure confirmed by executed market buys.
*/
func precursorScore(imbalance, buyPressure float64) float64 {
	if imbalance <= 0 || buyPressure <= 0 {
		return 0
	}

	bookSide := imbalance

	if bookSide > 1 {
		bookSide = 1
	}

	buySide := (buyPressure + 1) / 2

	return bookSide * buySide
}
