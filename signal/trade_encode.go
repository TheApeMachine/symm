package signal

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat"
)

const (
	grossHistoryCap = 64
	minGrossHistory = 8
)

/*
TradeWireProfile selects how Trade.Read encodes the scoped symbol window.
*/
type TradeWireProfile int

const (
	TradeWireEvent TradeWireProfile = iota
	TradeWireFlow
	TradeWireExcitation
)

/*
TradeFlowBatch is the aggregated CVD flow frame for one symbol window.
*/
type TradeFlowBatch struct {
	BuyNotional    float64
	SellNotional   float64
	TradeCount     float64
	GrossFloor     float64
	MedianNotional float64
	Prices         []float64
	Volume         float64
	Elapsed        float64
	Observed       time.Time
	Net            float64
}

func (trade *Trade) flowBatch(symbol string) (TradeFlowBatch, bool) {
	window, windowOK := trade.Window(symbol)

	if !windowOK {
		return TradeFlowBatch{}, false
	}

	var (
		buyNotional  float64
		sellNotional float64
		notionals    []float64
		tradeCount   int
	)

	trade.Scan(symbol, func(record *TradeRecord) {
		if record == nil {
			return
		}

		tradeCount++
		notional := record.Price * record.Qty
		notionals = append(notionals, notional)

		if record.Side == "buy" {
			buyNotional += notional
		}

		if record.Side == "sell" {
			sellNotional += notional
		}
	})

	gross := buyNotional + sellNotional

	if gross <= 0 || tradeCount < 2 || len(window.Prices) < 2 {
		return TradeFlowBatch{}, false
	}

	trade.recordGross(symbol, gross)

	return TradeFlowBatch{
		BuyNotional:    buyNotional,
		SellNotional:   sellNotional,
		TradeCount:     float64(tradeCount),
		GrossFloor:     trade.grossFloor(symbol),
		MedianNotional: statistic.MedianOf(notionals),
		Prices:         window.Prices,
		Volume:         window.Volume,
		Elapsed:        window.Elapsed,
		Observed:       window.Latest.Timestamp,
		Net:            buyNotional - sellNotional,
	}, true
}

func encodeFlowBatch(batch TradeFlowBatch) []byte {
	values := make([]float64, 0, 5+len(batch.Prices))
	values = append(values,
		batch.BuyNotional,
		batch.SellNotional,
		batch.TradeCount,
		batch.GrossFloor,
		batch.MedianNotional,
	)
	values = append(values, batch.Prices...)

	payload := make([]byte, 8*len(values))

	for index, sample := range values {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func (trade *Trade) excitationPayload(symbol string) ([]byte, time.Time, bool) {
	var (
		first     *TradeRecord
		latest    *TradeRecord
		buyTimes  []float64
		sellTimes []float64
	)

	trade.Scan(symbol, func(record *TradeRecord) {
		if record == nil {
			return
		}

		if first == nil {
			first = record
		}

		latest = record
		seconds := float64(record.Timestamp.UnixNano()) / float64(time.Second)

		switch record.Side {
		case "buy":
			buyTimes = append(buyTimes, seconds)
		case "sell":
			sellTimes = append(sellTimes, seconds)
		}
	})

	if latest == nil || len(buyTimes)+len(sellTimes) < 2 {
		return nil, time.Time{}, false
	}

	buyTimes, sellTimes = trimExcitationArrivals(
		buyTimes,
		sellTimes,
		MaxFeatureFloats(
			"trade",
			"trade",
			symbol,
			ExcitationPayloadHeader,
		),
	)

	if len(buyTimes)+len(sellTimes) < 2 {
		return nil, time.Time{}, false
	}

	horizon := float64(latest.Timestamp.UnixNano()) / float64(time.Second)
	windowSpan := latest.Timestamp.Sub(first.Timestamp)
	fitCooldown := algorithm.DeriveFitCooldown(windowSpan)

	values := make([]float64, 0, 4+len(buyTimes)+len(sellTimes))
	values = append(values,
		horizon,
		fitCooldown.Seconds(),
		float64(len(buyTimes)),
		float64(len(sellTimes)),
	)
	values = append(values, buyTimes...)
	values = append(values, sellTimes...)

	payload := make([]byte, 8*len(values))

	for index, sample := range values {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload, latest.Timestamp, true
}

func trimExcitationArrivals(
	buyTimes []float64,
	sellTimes []float64,
	maxSamples int,
) ([]float64, []float64) {
	total := ExcitationPayloadHeader + len(buyTimes) + len(sellTimes)

	if total <= maxSamples {
		return buyTimes, sellTimes
	}

	drop := total - maxSamples

	for drop > 0 {
		if len(buyTimes) >= len(sellTimes) && len(buyTimes) > 0 {
			buyTimes = buyTimes[1:]

			drop--

			continue
		}

		if len(sellTimes) > 0 {
			sellTimes = sellTimes[1:]

			drop--

			continue
		}

		break
	}

	return buyTimes, sellTimes
}

func (trade *Trade) recordGross(symbol string, gross float64) {
	if gross <= 0 {
		return
	}

	value, _ := trade.grossHistory.LoadOrStore(symbol, make([]float64, 0, grossHistoryCap))
	history := value.([]float64)
	history = append(history, gross)

	if len(history) > grossHistoryCap {
		history = history[len(history)-grossHistoryCap:]
	}

	trade.grossHistory.Store(symbol, history)
}

func (trade *Trade) grossFloor(symbol string) float64 {
	value, ok := trade.grossHistory.Load(symbol)

	if !ok {
		return 0
	}

	history := value.([]float64)

	if len(history) < minGrossHistory {
		return 0
	}

	return stat.Quantile(0.1, stat.LinInterp, history, nil)
}

func (trade *Trade) batchArtifact(scope string) *datura.Artifact {
	switch trade.WireProfile {
	case TradeWireFlow:
		batch, batchOK := trade.flowBatch(scope)

		if !batchOK {
			return nil
		}

		artifact := datura.Acquire("trade", datura.Artifact_Type_json)
		artifact.WithRole("trade")
		artifact.WithScope(scope)
		artifact.WithPayload(encodeFlowBatch(batch))

		return artifact
	case TradeWireExcitation:
		payload, _, payloadOK := trade.excitationPayload(scope)

		if !payloadOK {
			return nil
		}

		artifact := datura.Acquire("trade", datura.Artifact_Type_json)
		artifact.WithRole("trade")
		artifact.WithScope(scope)
		artifact.WithPayload(payload)

		return artifact
	default:
		return nil
	}
}
