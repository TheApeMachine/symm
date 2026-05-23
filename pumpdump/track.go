package pumpdump

import (
	"math"
	"sync"
	"time"
)

const (
	minVolumeHistory = 4
	minPriceHistory  = 3
	minSpreadHistory = 8
	volumeHistoryCap = 20
	bucketWindow     = 5 * time.Minute
	spreadHistoryCap = 20
	priceHistoryCap  = 20
)

/*
TrackStore holds per-symbol rolling windows for pump precursors.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack accumulates five-minute buckets used by the article trigger.
*/
type SymbolTrack struct {
	volumes         []float64
	spreads         []float64
	priceMoves      []float64
	bucketVolume    float64
	bucketOpenPrice float64
	lastPrice       float64
	dailyQuoteVol   float64
	bucketStart     time.Time
}

/*
NewTrackStore creates an empty track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
	}
}

/*
ApplyTicker ingests last price and 24h quote volume for liquidity filtering.
*/
func (trackStore *TrackStore) ApplyTicker(symbol string, last, volumeBase float64) {
	if symbol == "" || last <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.lastPrice = last
	track.dailyQuoteVol = volumeBase * last
}

/*
AddVolume adds executed trade volume into the active five-minute bucket.
*/
func (trackStore *TrackStore) AddVolume(symbol string, volume float64) {
	if symbol == "" || volume <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.bucketVolume += volume
}

/*
RecordSpread stores bid-ask spread samples for tightening detection.
*/
func (trackStore *TrackStore) RecordSpread(symbol string, spreadBPS float64) {
	if symbol == "" || spreadBPS <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.spreads = append(track.spreads, spreadBPS)

	if len(track.spreads) > spreadHistoryCap {
		track.spreads = track.spreads[len(track.spreads)-spreadHistoryCap:]
	}
}

/*
RollBuckets closes any elapsed five-minute windows.
*/
func (trackStore *TrackStore) RollBuckets(now time.Time) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	for _, track := range trackStore.bySymbol {
		track.roll(now)
	}
}

/*
PassesLiquidity keeps symbols below the live cross-section median daily quote volume.
*/
func (trackStore *TrackStore) PassesLiquidity(symbol string) bool {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.dailyQuoteVol <= 0 {
		return false
	}

	quoteVolumes := make([]float64, 0, len(trackStore.bySymbol))

	for _, candidate := range trackStore.bySymbol {
		if candidate.dailyQuoteVol <= 0 {
			continue
		}

		quoteVolumes = append(quoteVolumes, candidate.dailyQuoteVol)
	}

	if len(quoteVolumes) < 2 {
		return false
	}

	liquidityLine := crossSectionMedian(quoteVolumes)

	return track.dailyQuoteVol < liquidityLine
}

/*
VolumeSpike reports whether current bucket volume exceeds the symbol's own ratio fence.
*/
func (trackStore *TrackStore) VolumeSpike(symbol string) (float64, bool) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.volumes) < minVolumeHistory {
		return 0, false
	}

	baseline := mean(track.volumes)

	if baseline <= 0 || track.bucketVolume <= 0 {
		return 0, false
	}

	ratio := track.bucketVolume / baseline
	fence := volumeRatioFence(volumeRatios(track.volumes))

	if fence <= 0 {
		return 0, false
	}

	return ratio, ratio > fence
}

/*
PriceFlat reports whether the active bucket move is below the symbol's own move history.
*/
func (trackStore *TrackStore) PriceFlat(symbol string) bool {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.bucketOpenPrice <= 0 || track.lastPrice <= 0 {
		return false
	}

	if len(track.priceMoves) < minPriceHistory {
		return false
	}

	move := math.Abs(track.lastPrice/track.bucketOpenPrice - 1)
	quietLine := median(track.priceMoves)

	return move < quietLine
}

/*
SpreadTight reports sudden quote compression vs the spread history.
*/
func (trackStore *TrackStore) SpreadTight(symbol string, spreadBPS float64) bool {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.spreads) < minSpreadHistory {
		return false
	}

	medianSpread := median(track.spreads)

	return spreadBPS < medianSpread
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		volumes:    make([]float64, 0, volumeHistoryCap),
		spreads:    make([]float64, 0, spreadHistoryCap),
		priceMoves: make([]float64, 0, priceHistoryCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (track *SymbolTrack) roll(now time.Time) {
	if track.bucketStart.IsZero() {
		track.bucketStart = now
		track.bucketOpenPrice = track.lastPrice
		return
	}

	if now.Sub(track.bucketStart) < bucketWindow {
		return
	}

	if track.bucketOpenPrice > 0 && track.lastPrice > 0 {
		move := math.Abs(track.lastPrice/track.bucketOpenPrice - 1)
		track.priceMoves = append(track.priceMoves, move)

		if len(track.priceMoves) > priceHistoryCap {
			track.priceMoves = track.priceMoves[len(track.priceMoves)-priceHistoryCap:]
		}
	}

	if track.bucketVolume > 0 {
		track.volumes = append(track.volumes, track.bucketVolume)

		if len(track.volumes) > volumeHistoryCap {
			track.volumes = track.volumes[len(track.volumes)-volumeHistoryCap:]
		}
	}

	track.bucketVolume = 0
	track.bucketStart = now
	track.bucketOpenPrice = track.lastPrice
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64

	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return percentileSorted(cp, 0.5)
}

func sortFloats(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}
