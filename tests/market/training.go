package market

import (
	"context"
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

// TrainingPrice supplies the fixture's explicit 0.1 percent fee, not a policy default.
func TrainingPrice(ctx context.Context) *broker.Price {
	price := broker.NewPrice(ctx, nil, nil, nil, nil)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
	return price
}

// TrainingTape expands alternating opportunity legs into stable volume regimes.
// The repetitions and one-cent spread specify synthetic data, not detector policy.
func TrainingTape(legs int) []*data.Measurement {
	base := ImpulseTape("BTC/USD", legs)
	var frames []*data.Measurement
	arena := data.NewArenaOwner("training", 4096)

	for _, source := range base {
		sourcePeers := source.Peers()

		if len(sourcePeers) == 0 {
			continue
		}

		value := data.Pull(sourcePeers[0].Read("value")).Metric.Raw

		for repeat := range 32 {
			sequence := int64(len(frames) + 1)

			quoteMeta := []data.StringEntry{
				{Key: "venue", Value: "true"},
				{Key: "volume-unit", Value: "base"},
				{Key: "owner", Value: "quote"},
				{Key: "channel", Value: "ticker"},
			}

			bidVal := value
			bidExact := decimal.NewFromFloat64(bidVal)
			bidMetric := data.NewExactMetric("bid", bidExact, data.UnitPrice, data.TimescaleInstantaneous)
			bidMetric.Standardized = bidVal

			askVal := value + 0.01
			askExact := decimal.NewFromFloat64(askVal)
			askMetric := data.NewExactMetric("ask", askExact, data.UnitPrice, data.TimescaleInstantaneous)
			askMetric.Standardized = askVal

			quote := arena.NewMeasurement(
				1,
				"BTC/USD",
				"quote",
				sequence,
				sequence,
				nil,
				quoteMeta...,
			)
			quote.At = source.At
			quote.From = source.From
			quote.Write(bidMetric, askMetric)

			var tradeMetrics []data.Metric

			for entry := range sourcePeers[0].Read() {
				tradeMetrics = append(tradeMetrics, entry.Metric)
			}

			priceExact := decimal.NewFromFloat64(value)
			priceMetric := data.NewExactMetric("price", priceExact, data.UnitPrice, data.TimescaleInstantaneous)
			priceMetric.Standardized = value
			tradeMetrics = append(tradeMetrics, priceMetric)

			var tradeMeta []data.StringEntry

			for _, key := range []string{"venue", "volume-unit", "channel"} {
				if val := sourcePeers[0].Meta(key); val != "" {
					tradeMeta = append(tradeMeta, data.StringEntry{Key: key, Value: val})
				}
			}

			trade := arena.NewMeasurement(
				1,
				"BTC/USD",
				sourcePeers[0].Source,
				sequence,
				sequence,
				nil,
				tradeMeta...,
			)
			trade.At = source.At
			trade.From = source.From
			trade.Write(tradeMetrics...)

			peers := []*data.Measurement{quote, trade}

			for _, oldPeer := range sourcePeers[1:] {
				var peerMetrics []data.Metric

				for entry := range oldPeer.Read() {
					metric := entry.Metric

					if metric.Label == "value" {
						metric.Raw += float64(repeat%2) / 100
						metric.Standardized = metric.Raw
					}

					peerMetrics = append(peerMetrics, metric)
				}

				extraPeer := arena.NewMeasurement(
					1,
					"BTC/USD",
					oldPeer.Source,
					sequence,
					sequence,
					nil,
				)
				extraPeer.At = source.At
				extraPeer.From = source.From
				extraPeer.Write(peerMetrics...)
				peers = append(peers, extraPeer)
			}

			frameMeta := []data.StringEntry{
				{Key: "owner", Value: "training"},
				{Key: "fixture", Value: fmt.Sprint(sequence)},
			}

			prevInput := data.NewMetric("previous_input", float64(sequence-1), data.UnitCount, data.TimescaleInstantaneous)
			prevInput.Standardized = float64(sequence - 1)

			inputCount := data.NewMetric("input_count", 4, data.UnitCount, data.TimescaleInstantaneous)
			inputCount.Standardized = 4

			impulseVer := data.NewMetric("impulse_version", grid.FormatVersion, data.UnitDimensionless, data.TimescaleInstantaneous)
			impulseVer.Standardized = grid.FormatVersion

			frame := arena.NewMeasurement(
				1,
				"BTC/USD",
				"training",
				sequence,
				sequence,
				peers,
				frameMeta...,
			)
			frame.At = source.At
			frame.From = source.From
			frame.Write(prevInput, inputCount, impulseVer)

			frames = append(frames, frame)
		}
	}

	return frames
}
