package signal

import (
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"time"

	tes "github.com/theapemachine/symm/tests/types"
)

/*
Generate yields raw JSON []byte payload frames derived by updating the given template with signal steps.
*/
func (generator *Generator) Generate(template []byte) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		sample := generator.Step()
		frame := generator.Render(template, sample)

		if len(frame) > 0 {
			yield(frame)
		}
	}
}

/*
Render writes one already-sampled market state into a channel template.

Keeping sampling separate from rendering lets one venue tick publish the same
price, quantity, and timestamp through ticker, book, trade, and level3 rather
than advancing the simulated market once per channel.
*/
func (generator *Generator) Render(template []byte, sample tes.Sample) []byte {
	if len(template) == 0 {
		payload, err := json.Marshal(map[string]any{
			"channel": "ticker",
			"type":    "update",
			"data":    []tes.Sample{sample},
		})

		if err != nil {
			panic(fmt.Errorf("generator: encode ticker frame: %w", err))
		}

		return payload
	}

	var wire map[string]any

	if err := json.Unmarshal(template, &wire); err != nil {
		panic(fmt.Errorf("generator: decode channel template: %w", err))
	}

	channel, _ := wire["channel"].(string)
	wireType, _ := wire["type"].(string)
	stamp := sample.Timestamp.Format(time.RFC3339Nano)
	generator.sequence++

	if data, ok := wire["data"].([]any); ok && len(data) > 0 {
		if row, ok := data[0].(map[string]any); ok {
			row["symbol"] = sample.Symbol
			row["timestamp"] = stamp
			delete(row, "checksum")

			switch channel {
			case "book":
				row["bids"] = renderDepth(sample.Bids, sample.Bid, sample.BidQty)
				row["asks"] = renderDepth(sample.Asks, sample.Ask, sample.AskQty)
			case "level3":
				bids := resolvedDepth(sample.Bids, sample.Bid, sample.BidQty)
				asks := resolvedDepth(sample.Asks, sample.Ask, sample.AskQty)

				if wireType == "snapshot" {
					orderSymbol := strings.NewReplacer("/", "-", ".", "-").
						Replace(sample.Symbol)
					row["bids"] = renderLevel3Depth(
						bids, "OBID-"+orderSymbol, stamp,
					)
					row["asks"] = renderLevel3Depth(
						asks, "OASK-"+orderSymbol, stamp,
					)
					generator.l3Bids = copyDepth(bids)
					generator.l3Asks = copyDepth(asks)
					break
				}

				orderSymbol := strings.NewReplacer("/", "-", ".", "-").
					Replace(sample.Symbol)
				row["bids"] = renderLevel3Update(
					generator.l3Bids, bids, "OBID-"+orderSymbol, stamp,
				)
				row["asks"] = renderLevel3Update(
					generator.l3Asks, asks, "OASK-"+orderSymbol, stamp,
				)
				generator.l3Bids = copyDepth(bids)
				generator.l3Asks = copyDepth(asks)
			case "trade":
				row["side"] = sample.AggressorSide
				row["price"] = sample.Last

				/*
					A trade reports the quantity that actually executed, not
					the size resting on the bid. Ignition scores volume rate
					against its own median, so the executed quantity is the
					only figure that carries the surge.
				*/
				row["qty"] = sample.StepVolume
				row["ord_type"] = "limit"
				row["trade_id"] = generator.sequence
			default:
				row["bid"] = sample.Bid
				row["bid_qty"] = sample.BidQty
				row["ask"] = sample.Ask
				row["ask_qty"] = sample.AskQty
				row["last"] = sample.Last
				row["volume"] = sample.Volume
				row["vwap"] = sample.VWAP
				row["low"] = sample.Low
				row["high"] = sample.High
				row["change"] = sample.Change
				row["change_pct"] = sample.ChangePct
			}

			wire["data"] = []any{row}
		}
	}

	if wireType != "snapshot" {
		wire["type"] = "update"
	}
	payload, err := json.Marshal(wire)

	if err != nil {
		panic(fmt.Errorf("generator: encode channel frame: %w", err))
	}

	return payload
}

func renderDepth(
	levels []tes.DepthLevel,
	topPrice float64,
	topQuantity float64,
) []any {
	if len(levels) == 0 {
		return []any{map[string]any{
			"price": topPrice, "qty": topQuantity,
		}}
	}

	rows := make([]any, len(levels))

	for index, level := range levels {
		rows[index] = map[string]any{
			"price": level.Price, "qty": level.Quantity,
		}
	}

	return rows
}

func resolvedDepth(
	levels []tes.DepthLevel,
	topPrice float64,
	topQuantity float64,
) []tes.DepthLevel {
	if len(levels) > 0 {
		return levels
	}

	return []tes.DepthLevel{{Price: topPrice, Quantity: topQuantity}}
}

func renderLevel3Depth(
	levels []tes.DepthLevel,
	orderPrefix string,
	stamp string,
) []any {
	rows := make([]any, len(levels))

	for index, level := range levels {
		rows[index] = map[string]any{
			"event":       "add",
			"order_id":    fmt.Sprintf("%s-%d", orderPrefix, index),
			"limit_price": level.Price,
			"order_qty":   level.Quantity,
			"timestamp":   stamp,
		}
	}

	return rows
}

func renderLevel3Update(
	previous []tes.DepthLevel,
	current []tes.DepthLevel,
	orderPrefix string,
	stamp string,
) []any {
	rows := make([]any, 0, len(previous)+len(current))

	for index, level := range previous {
		rows = append(rows, map[string]any{
			"event":       "delete",
			"order_id":    fmt.Sprintf("%s-%d", orderPrefix, index),
			"limit_price": level.Price,
			"order_qty":   level.Quantity,
			"timestamp":   stamp,
		})
	}

	for index, level := range current {
		rows = append(rows, map[string]any{
			"event":       "add",
			"order_id":    fmt.Sprintf("%s-%d", orderPrefix, index),
			"limit_price": level.Price,
			"order_qty":   level.Quantity,
			"timestamp":   stamp,
		})
	}

	return rows
}

func copyDepth(source []tes.DepthLevel) []tes.DepthLevel {
	return append([]tes.DepthLevel{}, source...)
}
