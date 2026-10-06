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
// The repetitions specify synthetic data, not detector policy. Each frame
// carries one spot:trade peer followed by the opposed signal peers.
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

			// The pipeline carries spot:trade frames only: exact price, qty,
			// and aggressor side. No touch is fabricated here; a consumer
			// that needs one reads a seeded BookManager.
			qty := data.Pull(sourcePeers[0].Read("qty")).Metric
			side := "buy"

			if repeat%2 != 0 {
				side = "sell"
			}

			priceMetric := data.NewExactMetric(
				"price", decimal.NewFromFloat64(value), data.UnitPrice, data.TimescaleInstantaneous,
			)
			priceMetric.Standardized = value

			qtyMetric := data.NewExactMetric("qty", qty.Exact, data.UnitQuantity, data.TimescaleInstantaneous)
			qtyMetric.Standardized = qty.Standardized

			trade := arena.NewMeasurement(
				1,
				"BTC/USD",
				"spot:trade",
				sequence,
				sequence,
				nil,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: side},
			)
			trade.At = source.At
			trade.From = source.From
			trade.Write(priceMetric, qtyMetric)

			peers := []*data.Measurement{trade}

			for _, oldPeer := range sourcePeers[1:] {
				var peerMetrics []*data.Metric

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

			frameMeta := []*data.StringEntry{
				{Key: "owner", Value: "training"},
				{Key: "fixture", Value: fmt.Sprint(sequence)},
			}

			prevInput := data.NewMetric("previous_input", float64(sequence-1), data.UnitCount, data.TimescaleInstantaneous)
			prevInput.Standardized = float64(sequence - 1)

			inputCount := data.NewMetric("input_count", float64(len(peers)), data.UnitCount, data.TimescaleInstantaneous)
			inputCount.Standardized = float64(len(peers))

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
