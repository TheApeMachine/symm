package pumpdump

import (
	"math"
	"sort"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
)

type priorMeasurement struct {
	stamp       float64
	volume      float64
	last        float64
	volumeDelta float64
	logReturn   float64
	spread      float64
	bookSpread  float64
	tradeVolume float64
	touchDepth  float64
	rvol        float64
	compression float64
	decline     float64
	exhaustion  float64
}

type measurementHistory struct {
	volumeDeltas     []float64
	logReturns       []float64
	spreads          []float64
	bookSpreads      []float64
	tradeVolumes     []float64
	touchDepths      []float64
	lasts            []float64
	lifts            []float64
	stamps           []float64
	prevVolume       float64
	prevLast         float64
	prevRVOL         float64
	prevTradeVolume  float64
	ignitionFloor    float64
	compressionFloor float64
	declineFloor     float64
	exhaustionStamp  float64
}

func (signal *Signal) history(symbol string) measurementHistory {
	history := measurementHistory{}

	if signal.tree == nil {
		return history
	}

	samples := make([]priorMeasurement, 0)

	for prior := range signal.tree.Seek(measurementPrefix(symbol)) {
		sample := readPriorMeasurement(prior)

		if sample.stamp <= 0 {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return history
	}

	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	stamps := make([]float64, len(samples))

	for sampleIndex := range samples {
		stamps[sampleIndex] = samples[sampleIndex].stamp
	}

	keep := statutil.WindowDepth(stamps)

	if keep <= 0 {
		return history
	}

	if keep < len(samples) {
		samples = samples[len(samples)-keep:]
		stamps = stamps[len(stamps)-keep:]
	}

	history.stamps = stamps
	latest := samples[len(samples)-1]
	history.prevVolume = latest.volume
	history.prevLast = latest.last
	history.prevRVOL = latest.rvol
	history.prevTradeVolume = latest.tradeVolume

	for _, sample := range samples {
		history.volumeDeltas = append(history.volumeDeltas, sample.volumeDelta)
		history.logReturns = append(history.logReturns, sample.logReturn)
		history.spreads = append(history.spreads, sample.spread)
		history.bookSpreads = append(history.bookSpreads, sample.bookSpread)
		history.tradeVolumes = append(history.tradeVolumes, sample.tradeVolume)
		history.touchDepths = append(history.touchDepths, sample.touchDepth)
		history.lasts = append(history.lasts, sample.last)
		history.lifts = append(history.lifts, math.Max(0, sample.rvol-1))

		if sample.exhaustion > history.exhaustionStamp {
			history.exhaustionStamp = sample.exhaustion
		}
	}

	history.ignitionFloor = liftMedian(samples)
	history.compressionFloor = compressionMedian(samples)
	history.declineFloor = declineMedian(samples)

	return history
}

func readPriorMeasurement(prior *datura.Artifact) priorMeasurement {
	stamp := datura.Peek[float64](prior, "timestamp")

	if stamp <= 0 && prior != nil {
		stamp = float64(prior.Timestamp())
	}

	return priorMeasurement{
		stamp:       stamp,
		volume:      datura.Peek[float64](prior, "volume"),
		last:        datura.Peek[float64](prior, "last"),
		volumeDelta: datura.Peek[float64](prior, "volumeDelta"),
		logReturn:   datura.Peek[float64](prior, "logReturn"),
		spread:      datura.Peek[float64](prior, "spread"),
		bookSpread:  datura.Peek[float64](prior, "bookSpread"),
		tradeVolume: datura.Peek[float64](prior, "tradeVolume"),
		touchDepth:  datura.Peek[float64](prior, "touchDepth"),
		rvol:        datura.Peek[float64](prior, "output", "rvol"),
		compression: datura.Peek[float64](prior, "output", "compression"),
		decline:     datura.Peek[float64](prior, "output", "rvolDecline"),
		exhaustion:  datura.Peek[float64](prior, "lastExhaustionStamp"),
	}
}

func measurementPrefix(symbol string) []byte {
	return []byte("measurement/" + symbol + "/" + string(logic.SourcePumpDump) + "/")
}

func positiveSamples(values []float64) []float64 {
	positive := make([]float64, 0, len(values))

	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	return positive
}

func liftMedian(samples []priorMeasurement) float64 {
	lifts := make([]float64, 0, len(samples))

	for _, sample := range samples {
		if sample.rvol <= 0 {
			continue
		}

		lifts = append(lifts, math.Max(0, sample.rvol-1))
	}

	return statutil.Median(lifts)
}

func compressionMedian(samples []priorMeasurement) float64 {
	compressions := make([]float64, 0, len(samples))

	for _, sample := range samples {
		if sample.compression > 0 {
			compressions = append(compressions, sample.compression)
		}
	}

	return statutil.Median(compressions)
}

func declineMedian(samples []priorMeasurement) float64 {
	declines := make([]float64, 0, len(samples))

	for _, sample := range samples {
		if sample.decline > 0 {
			declines = append(declines, sample.decline)
		}
	}

	return statutil.Median(declines)
}

func geometricMean(first, second float64) float64 {
	if first <= 0 || second <= 0 {
		return math.Max(first, second)
	}

	return math.Sqrt(first * second)
}

func rvolDecline(
	rvol, tradeVolume float64,
	history measurementHistory,
) float64 {
	lift := math.Max(0, rvol-1)
	prevLift := math.Max(0, history.prevRVOL-1)
	decline := 0.0

	if prevLift > lift {
		decline = (prevLift - lift) / (1 + prevLift)
	}

	if history.prevTradeVolume <= 0 || history.prevTradeVolume <= tradeVolume {
		return decline
	}

	tradeDecline := (history.prevTradeVolume - tradeVolume) / (1 + history.prevTradeVolume)

	if tradeDecline > decline {
		return tradeDecline
	}

	return decline
}
