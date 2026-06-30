package pumpdump

import (
	"math"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/statutil"
)

type bookSnapshot struct {
	spread     float64
	touchDepth float64
	stamp      float64
}

type tradeSnapshot struct {
	volume float64
	stamp  float64
}

func (signal *Signal) bookEnrichment(symbol string, currentStamp float64) bookSnapshot {
	if signal.tree == nil {
		return bookSnapshot{}
	}

	if snapshot := latestBookSnapshot(signal, symbol, currentStamp); snapshot.spread > 0 {
		return snapshot
	}

	snapshot := latestBook(signal, symbol, currentStamp, scopedRolePrefix("book", symbol, 0, currentStamp))

	if snapshot.spread > 0 {
		return snapshot
	}

	return latestBook(signal, symbol, currentStamp, []byte("book/"+symbol+"/"))
}

func latestBookSnapshot(signal *Signal, symbol string, currentStamp float64) bookSnapshot {
	raw, ok := signal.tree.Get(latestScopedKey("book", symbol))

	if !ok || len(raw) == 0 {
		return bookSnapshot{}
	}

	artifact := &datura.Artifact{}

	if _, err := artifact.Unpack(raw); err != nil {
		return bookSnapshot{}
	}

	stamp := float64(artifact.Timestamp())

	if currentStamp > 0 && stamp > currentStamp {
		return bookSnapshot{}
	}

	for rowIndex := 0; ; rowIndex++ {
		if datura.Peek[string](artifact, "data", rowIndex, "symbol") == "" {
			return bookSnapshot{}
		}

		if datura.Peek[string](artifact, "data", rowIndex, "symbol") != symbol {
			continue
		}

		return readBookRow(artifact, rowIndex, stamp)
	}
}

func (signal *Signal) tradeEnrichment(
	symbol string,
	windowStamps []float64,
	currentStamp float64,
) tradeSnapshot {
	if signal.tree == nil {
		return tradeSnapshot{}
	}

	windowStart := tradeWindowStart(windowStamps, currentStamp)
	prefix := scopedRolePrefix("trade", symbol, windowStart, currentStamp)
	snapshot := tradeVolume(signal, symbol, windowStart, currentStamp, prefix)

	if snapshot.volume > 0 {
		return snapshot
	}

	return tradeVolume(signal, symbol, windowStart, currentStamp, []byte("trade/"+symbol+"/"))
}

func latestBook(
	signal *Signal,
	symbol string,
	currentStamp float64,
	prefix []byte,
) bookSnapshot {
	latest := bookSnapshot{}

	for artifact := range signal.tree.Seek(prefix) {
		stamp := float64(artifact.Timestamp())

		if currentStamp > 0 && stamp > currentStamp {
			break
		}

		for rowIndex := 0; ; rowIndex++ {
			if datura.Peek[string](artifact, "data", rowIndex, "symbol") == "" {
				break
			}

			if datura.Peek[string](artifact, "data", rowIndex, "symbol") != symbol {
				continue
			}

			snapshot := readBookRow(artifact, rowIndex, stamp)

			if snapshot.spread <= 0 || snapshot.stamp < latest.stamp {
				continue
			}

			latest = snapshot
		}
	}

	return latest
}

func tradeVolume(
	signal *Signal,
	symbol string,
	windowStart, currentStamp float64,
	prefix []byte,
) tradeSnapshot {
	snapshot := tradeSnapshot{}

	for artifact := range signal.tree.Seek(prefix) {
		stamp := float64(artifact.Timestamp())

		if currentStamp > 0 && stamp > currentStamp {
			break
		}

		if windowStart > 0 && stamp < windowStart {
			continue
		}

		for rowIndex := 0; ; rowIndex++ {
			if datura.Peek[string](artifact, "data", rowIndex, "symbol") == "" {
				break
			}

			if datura.Peek[string](artifact, "data", rowIndex, "symbol") != symbol {
				continue
			}

			quantity := datura.Peek[float64](artifact, "data", rowIndex, "qty")

			if quantity <= 0 {
				continue
			}

			snapshot.volume += quantity
			snapshot.stamp = stamp
		}
	}

	return snapshot
}

func readBookRow(artifact *datura.Artifact, rowIndex int, stamp float64) bookSnapshot {
	bidPrice := datura.Peek[float64](artifact, "data", rowIndex, "bids", 0, "price")
	askPrice := datura.Peek[float64](artifact, "data", rowIndex, "asks", 0, "price")

	if bidPrice <= 0 || askPrice <= 0 || askPrice < bidPrice {
		return bookSnapshot{}
	}

	bidQuantity := datura.Peek[float64](artifact, "data", rowIndex, "bids", 0, "qty")
	askQuantity := datura.Peek[float64](artifact, "data", rowIndex, "asks", 0, "qty")

	return bookSnapshot{
		spread:     askPrice - bidPrice,
		touchDepth: math.Max(0, bidQuantity) + math.Max(0, askQuantity),
		stamp:      stamp,
	}
}

func peerRVOL(volume float64, crossSection *market.CrossSection) float64 {
	if crossSection == nil {
		return 0
	}

	return statutil.ScaleByMedianOrUnity(volume, positiveSamples(crossSection.Volumes()))
}

func peerPrecursor(
	symbol string,
	logReturn float64,
	crossSection *market.CrossSection,
) float64 {
	if crossSection == nil {
		return 0
	}

	_, _, _, peerEnergyMedian, ok := crossSection.SymbolPeerStats(
		symbol,
		crossSection.MinBarsRequired(),
	)

	if !ok {
		return statutil.ScaleByMedianOrUnity(logReturn, nil)
	}

	return statutil.ScaleByMedianOrUnity(logReturn, []float64{peerEnergyMedian})
}

func rolePrefix(role string, windowStart, currentStamp float64) []byte {
	if currentStamp <= 0 {
		return []byte(role + "/")
	}

	startStamp := currentStamp

	if windowStart > 0 {
		startStamp = windowStart
	}

	startCursor := hourCursor(startStamp)
	currentCursor := hourCursor(currentStamp)

	if startCursor == "" || startCursor != currentCursor {
		return []byte(role + "/")
	}

	return []byte(role + "/" + currentCursor)
}

func scopedRolePrefix(role, symbol string, windowStart, currentStamp float64) []byte {
	if symbol == "" {
		return rolePrefix(role, windowStart, currentStamp)
	}

	if currentStamp <= 0 {
		return []byte(role + "/" + symbol + "/")
	}

	startStamp := currentStamp

	if windowStart > 0 {
		startStamp = windowStart
	}

	startCursor := hourCursor(startStamp)
	currentCursor := hourCursor(currentStamp)

	if startCursor == "" || startCursor != currentCursor {
		return []byte(role + "/" + symbol + "/")
	}

	return []byte(role + "/" + symbol + "/" + currentCursor)
}

func latestScopedKey(role, symbol string) []byte {
	return []byte("latest/" + role + "/" + symbol)
}

func hourCursor(stamp float64) string {
	parts := strings.Split(datura.FormatTimestamp(int64(stamp)), "/")

	if len(parts) < 4 {
		return ""
	}

	return strings.Join(parts[:4], "/")
}

func tradeWindowStart(windowStamps []float64, currentStamp float64) float64 {
	if len(windowStamps) == 0 || currentStamp <= 0 {
		return 0
	}

	cadence := statutil.MedianCadence(windowStamps)

	if cadence <= 0 {
		return windowStamps[0]
	}

	depth := statutil.WindowDepth(windowStamps)

	if depth <= 0 {
		return windowStamps[0]
	}

	return currentStamp - cadence*float64(depth)
}
