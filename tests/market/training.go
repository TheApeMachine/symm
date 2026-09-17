package market

import (
	"context"
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/venue"
)

// TrainingPrice supplies the fixture's explicit 0.1 percent fee, not a policy default.
func TrainingPrice(ctx context.Context) *broker.Price {
	conn := &venue.Conn{}
	api := websocket.NewAPI(ctx, conn, conn, &websocket.FuturesLive{})
	price := broker.NewPrice(ctx, api, &broker.Instrument{})
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
	return price
}

// TrainingTape expands alternating opportunity legs into stable volume regimes.
// The repetitions and one-cent spread specify synthetic data, not detector policy.
func TrainingTape(legs int) []*data.Measurement[float64] {
	base := ImpulseTape("BTC/USD", legs)
	var frames []*data.Measurement[float64]

	for _, source := range base {
		for repeat := 0; repeat < 32; repeat++ {
			sequence := int64(len(frames) + 1)
			frame := source.Clone()
			frame.SeqIdx = sequence
			frame.Source = "training"
			frame.Provenance["owner"] = "training"
			frame.Metrics = map[string]data.Metric[float64]{
				"previous_input": {Raw: float64(sequence - 1)},
				"input_count":    {Raw: 4}, "impulse_version": {Raw: 1.0},
			}
			for index, peer := range frame.Peers {
				frame.Peers[index] = peer.Clone()
				frame.Peers[index].SeqIdx = sequence
			}
			trade := frame.Peers[0]
			value := trade.Metrics["value"].Raw
			trade.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: value, Exact: decimal.NewFromFloat64(value)}
			quote := data.NewMeasurement[float64]("quote", nil)
			quote.SeqIdx, quote.Label = sequence, "BTC/USD"
			quote.Metadata["venue"], quote.Metadata["volume-unit"] = "true", "base"
			quote.Provenance["owner"], quote.Provenance["channel"] = "quote", "ticker"
			for _, side := range []string{"bid", "ask"} {
				amount := value
				if side == "ask" {
					amount += 0.01
				}
				quote.Metrics[side] = data.Metric[float64]{Label: side, Raw: amount, Exact: decimal.NewFromFloat64(amount)}
			}
			frame.Peers = append([]*data.Measurement[float64]{quote}, frame.Peers...)
			// Changing features within a price regime keeps region contexts observable.
			for _, peer := range frame.Peers[2:] {
				metric := peer.Metrics["value"]
				metric.Raw += float64(repeat%2) / 100
				peer.Metrics["value"] = metric
			}
			frame.Metadata["fixture"] = fmt.Sprint(sequence)
			frames = append(frames, frame)
		}
	}
	return frames
}
