package pumpdump

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

type volumeTick struct {
	at     time.Time
	volume float64
}

const (
	volumeHistoryCap = 20
	bucketWindow     = 5 * time.Minute
	spreadHistoryCap = 20
	priceHistoryCap  = 20
)

/*
SymbolTrack holds rolling market state for one pair.
*/
type SymbolTrack struct {
	volumes           []float64
	spreads           []float64
	priceMoves        []float64
	confidenceHistory []float64
	rollingTicks      []volumeTick
	bucketVolume      float64
	bucketOpenPrice   float64
	lastPrice         float64
	dailyQuoteVol     float64
	bucketStart       time.Time
	lastRollAt        time.Time
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
TrackStore holds per-symbol rolling windows and listens on market broadcast groups.
*/
type TrackStore struct {
	engine.GaugeScan
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Pool
	subscriptions map[string]*qpool.Subscriber
	bySymbol      map[string]*SymbolTrack
}

/*
NewTrackStore subscribes to tick, trade, and book on the shared broadcast groups.
*/
func NewTrackStore(
	ctx context.Context,
	tick, trade, book *qpool.BroadcastGroup,
) (*TrackStore, error) {
	if tick == nil || trade == nil || book == nil {
		return nil, fmt.Errorf("track store requires tick, trade, and book groups")
	}

	ctx, cancel := context.WithCancel(ctx)

	trackStore := &TrackStore{
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string]*qpool.Subscriber),
		bySymbol:      make(map[string]*SymbolTrack),
	}

	trackStore.subscriptions["tick"] = tick.Subscribe("pumpdump:track", 65536)
	trackStore.subscriptions["trade"] = trade.Subscribe("pumpdump:track", 65536)
	trackStore.subscriptions["book"] = book.Subscribe("pumpdump:track", 65536)

	return trackStore, errnie.Require(map[string]any{
		"ctx":           ctx,
		"cancel":        cancel,
		"subscriptions": trackStore.subscriptions,
	})
}

func (trackStore *TrackStore) Tick() bool {
	select {
	case <-trackStore.ctx.Done():
		return false
	case value := <-trackStore.subscriptions["tick"].Incoming:
		if value == nil {
			return false
		}

		trackStore.applyTick(value)

		return true
	case value := <-trackStore.subscriptions["trade"].Incoming:
		if value == nil {
			return false
		}

		trackStore.applyTrade(value)

		return true
	case value := <-trackStore.subscriptions["book"].Incoming:
		if value == nil {
			return false
		}

		trackStore.applyBook(value)

		return true
	default:
		return false
	}
}

/*
ResetLiveScores clears per-tick gauge scores before the next measure pass.
*/
func (trackStore *TrackStore) ResetLiveScores() {
	trackStore.ResetGaugeScan()

	for _, track := range trackStore.bySymbol {
		track.liveScore = 0
	}
}

/*
RollBuckets closes any elapsed five-minute windows.
*/
func (trackStore *TrackStore) RollBuckets(now time.Time) {
	for _, track := range trackStore.bySymbol {
		track.roll(now)
		track.pruneRolling(now)
		track.bucketVolume = track.rollingVolume()
	}
}

/*
ApplyPredictionFeedback updates precursor calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	track := trackStore.ensure(feedback.Symbol)
	track.calibrator.Apply(feedback)
}

/*
CalibrationScale returns the live precursor multiplier for one symbol.
*/
func (trackStore *TrackStore) CalibrationScale(symbol string) float64 {
	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 1
	}

	return track.calibrator.Scale()
}

/*
RecordConfidence appends one raw confidence sample to symbol history.
*/
func (trackStore *TrackStore) RecordConfidence(symbol string, confidence float64) {
	if confidence <= 0 {
		return
	}

	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > volumeHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-volumeHistoryCap:]
	}
}

/*
SetLiveScore stores the latest normalized gauge reading for one symbol.
*/
func (trackStore *TrackStore) SetLiveScore(symbol string, score float64) {
	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return
	}

	track.liveScore = score
}

/*
SymbolLiveScore returns the latest normalized gauge reading for one symbol.
*/
func (trackStore *TrackStore) SymbolLiveScore(symbol string) float64 {
	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 0
	}

	return track.liveScore
}

func (trackStore *TrackStore) applyTick(value *qpool.QValue[any]) {
	update, ok := value.Value.(engine.TickUpdate)

	if !ok || update.Symbol == "" || update.Last <= 0 {
		return
	}

	track := trackStore.ensure(update.Symbol)
	track.lastPrice = update.Last
	track.dailyQuoteVol = update.VolumeBase * update.Last
}

func (trackStore *TrackStore) applyTrade(value *qpool.QValue[any]) {
	update, ok := value.Value.(engine.TradeUpdate)

	if !ok || update.Symbol == "" || update.BatchVolume <= 0 {
		return
	}

	track := trackStore.ensure(update.Symbol)
	track.rollingTicks = append(track.rollingTicks, volumeTick{
		at:     update.UpdatedAt,
		volume: update.BatchVolume,
	})
	track.pruneRolling(update.UpdatedAt)
	track.bucketVolume = track.rollingVolume()
}

func (trackStore *TrackStore) applyBook(value *qpool.QValue[any]) {
	update, ok := value.Value.(engine.BookUpdate)

	if !ok || update.Symbol == "" || update.SpreadBPS <= 0 {
		return
	}

	track := trackStore.ensure(update.Symbol)
	track.spreads = append(track.spreads, update.SpreadBPS)

	if len(track.spreads) > spreadHistoryCap {
		track.spreads = track.spreads[len(track.spreads)-spreadHistoryCap:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		volumes:           make([]float64, 0, volumeHistoryCap),
		spreads:           make([]float64, 0, spreadHistoryCap),
		priceMoves:        make([]float64, 0, priceHistoryCap),
		confidenceHistory: make([]float64, 0, volumeHistoryCap),
		calibrator:        engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (track *SymbolTrack) roll(now time.Time) {
	if track.bucketStart.IsZero() {
		track.bucketStart = now
		track.bucketOpenPrice = track.lastPrice
		track.lastRollAt = now

		return
	}

	if now.Sub(track.bucketStart) < bucketWindow {
		return
	}

	if track.bucketOpenPrice > 0 && track.lastPrice > 0 {
		move := stats.AbsRelativeMove(track.lastPrice, track.bucketOpenPrice)
		track.priceMoves = append(track.priceMoves, move)

		if len(track.priceMoves) > priceHistoryCap {
			track.priceMoves = track.priceMoves[len(track.priceMoves)-priceHistoryCap:]
		}
	}

	closedVolume := track.rollingVolume()

	if closedVolume > 0 {
		track.volumes = append(track.volumes, closedVolume)

		if len(track.volumes) > volumeHistoryCap {
			track.volumes = track.volumes[len(track.volumes)-volumeHistoryCap:]
		}
	}

	track.bucketStart = now
	track.bucketOpenPrice = track.lastPrice
	track.lastRollAt = now
}

func (track *SymbolTrack) pruneRolling(now time.Time) {
	cutoff := now.Add(-bucketWindow)
	kept := track.rollingTicks[:0]

	for _, tick := range track.rollingTicks {
		if tick.at.Before(cutoff) {
			continue
		}

		kept = append(kept, tick)
	}

	track.rollingTicks = kept
}

func (track *SymbolTrack) rollingVolume() float64 {
	total := 0.0

	for _, tick := range track.rollingTicks {
		total += tick.volume
	}

	return total
}
