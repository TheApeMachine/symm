package cmd

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
subscribeAck is the venue's acknowledgement of a subscribe request.
*/
type subscribeAck struct {
	Method  string `json:"method"`
	Success *bool  `json:"success"`
	Error   string `json:"error"`
	Result  struct {
		Channel string `json:"channel"`
		Symbol  string `json:"symbol"`
	} `json:"result"`
}

/*
subscribeRejection returns an error when buf is a subscribe acknowledgement
the venue rejected (success=false), and nil for every other frame. Order and
other method acks are not subscriptions and are left to their own handlers.
*/
func subscribeRejection(buf []byte) error {
	var ack subscribeAck

	if err := sonic.Unmarshal(buf, &ack); err != nil {
		return nil
	}

	if ack.Method != "subscribe" || ack.Success == nil || *ack.Success {
		return nil
	}

	return errnie.Err(
		errnie.NotAcceptable,
		fmt.Sprintf(
			"[ingress] subscribe rejected (channel=%q symbol=%q): %s",
			ack.Result.Channel, ack.Result.Symbol, ack.Error,
		),
		nil,
	)
}

/*
handleLevel3 decodes a Kraken Level3 frame, updates the book, and pushes individual
order event measurements to the store tee.
*/
func handleLevel3(
	buf []byte,
	epoch int64,
	book *broker.Book,
	storeTee nmruntime.Tee,
	name string,
) error {
	level3Msg, err := kraken.NewLevel3(buf)

	if err == nil {
		err = book.Update(level3Msg)
	}

	if err != nil {
		return errnie.Err(
			errnie.Internal,
			fmt.Sprintf("symm: %s level3 book update failed", name),
			err,
		)
	}

	for _, level3Data := range level3Msg.Data {
		for sideIdx, orders := range [][]kraken.Level3Order{level3Data.Bids, level3Data.Asks} {
			side := "bid"

			if sideIdx == 1 {
				side = "ask"
			}

			checksumStr := strconv.FormatInt(int64(level3Data.Checksum), 10)

			tick := system.Tick.Load()

			if tick <= 0 {
				tick = 1
			}

			for _, order := range orders {
				if order.Timestamp.IsZero() {
					return errnie.Err(
						errnie.Validation,
						fmt.Sprintf("symm: %s level3 order without timestamp", name),
						nil,
					)
				}

				measurement := data.NewMeasurement(
					epoch,
					level3Data.Symbol,
					"spot:level3",
					system.SeqIdx.Add(1),
					tick,
					&data.StringEntry{
						Key:   "type",
						Value: level3Data.Type,
					},
					&data.StringEntry{
						Key:   "order_id",
						Value: order.OrderID,
					},
					&data.StringEntry{
						Key:   "side",
						Value: side,
					},
					&data.StringEntry{
						Key:   "event",
						Value: order.Event,
					},
					&data.StringEntry{
						Key:   "checksum",
						Value: checksumStr,
					},
				)

				measurement.At = order.Timestamp.UTC()
				measurement.From = measurement.At

				measurement.Write(
					data.NewMetric(
						"checksum",
						float64(level3Data.Checksum),
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
					),
					data.NewExactMetric(
						"limit_price",
						order.LimitPrice,
						data.UnitCurrency,
						data.TimescaleInstantaneous,
					),
					data.NewExactMetric(
						"order_qty",
						order.OrderQty,
						data.UnitVolume,
						data.TimescaleInstantaneous,
					),
				)

				if storeTee != nil {
					storeTee.Push(measurement)
				}
			}
		}
	}

	return nil
}

/*
handleTrade decodes a Kraken trade frame, updates price, steps the workspace pipeline,
and pushes the trade measurement to the store tee.
*/
func handleTrade(
	buf []byte,
	epoch int64,
	price *broker.Price,
	workspace *nmruntime.Workspace,
	storeTee nmruntime.Tee,
	name string,
	afterTrade func(),
) error {
	tradeMsg, err := kraken.NewTrade(buf)

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("symm: %s trade frame undecodable", name),
			err,
		)
	}

	if !tradeMsg.IsSuccess() {
		return nil
	}

	for _, tradeItem := range tradeMsg.Data {
		price.Update(&tradeItem)

		if tradeItem.Timestamp.IsZero() {
			return errnie.Err(
				errnie.Validation,
				fmt.Sprintf("symm: %s trade without timestamp", name),
				nil,
			)
		}

		measurement := data.NewMeasurement(
			epoch,
			tradeItem.Symbol,
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
			&data.StringEntry{
				Key:   "type",
				Value: "trade",
			},
			&data.StringEntry{
				Key:   "ord_type",
				Value: tradeItem.OrderType,
			},
			&data.StringEntry{
				Key:   "trade_id",
				Value: strconv.FormatInt(tradeItem.TradeID, 10),
			},
			&data.StringEntry{
				Key:   "side",
				Value: tradeItem.Side,
			},
		)

		measurement.At = tradeItem.Timestamp.UTC()
		measurement.From = measurement.At

		measurement.Write(
			data.NewExactMetric(
				"price",
				&tradeItem.Price,
				data.UnitCurrency,
				data.TimescaleInstantaneous,
			),
			data.NewMetric(
				"qty",
				tradeItem.Qty,
				data.UnitVolume,
				data.TimescaleInstantaneous,
			),
		)

		if workspace != nil {
			workspace.Step(measurement)
		}

		if storeTee != nil {
			storeTee.Push(measurement)
		}

		if afterTrade != nil {
			afterTrade()
		}
	}

	return nil
}
