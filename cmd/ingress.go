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
	Channel string `json:"channel"`
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

	// A rate-limit rejection names its channel at the top level and has no
	// result object.
	if ack.Result.Channel == "" {
		ack.Result.Channel = ack.Channel
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
handleLevel3 decodes a Kraken Level3 frame, applies it to the book, and pushes
the orders of every entry the book accepted to the store tee, numbered with the
sequence the book took while it held the entry's write lock. An entry the book
did not accept never became live book state and is not stored, and accepted
entries are stored even when a later entry of the frame fails, so the stored
tape replays in SeqIdx order to exactly the live book.
*/
func handleLevel3(
	buf []byte,
	epoch int64,
	book *broker.Book,
	storeTee nmruntime.Tee,
	name string,
) error {
	level3Msg, err := kraken.NewLevel3(buf)

	if err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("symm: %s level3 frame undecodable", name),
			err,
		)
	}

	updateErr := book.Update(level3Msg)

	if storeTee != nil {
		if err := storeLevel3(level3Msg, epoch, storeTee, name); err != nil {
			return err
		}
	}

	if updateErr != nil {
		return errnie.Err(
			errnie.Internal,
			fmt.Sprintf("symm: %s level3 book update failed", name),
			updateErr,
		)
	}

	return nil
}

/*
storeLevel3 pushes one measurement per order of each accepted entry, bids then
asks, under consecutive sequence numbers from the entry's Sequence.
*/
func storeLevel3(level3Msg *kraken.Level3, epoch int64, storeTee nmruntime.Tee, name string) error {
	tick := system.Tick.Load()

	if tick <= 0 {
		tick = 1
	}

	for _, level3Data := range level3Msg.Data {
		if level3Data.Sequence == 0 {
			continue
		}

		sequence := level3Data.Sequence
		checksumStr := strconv.FormatInt(int64(level3Data.Checksum), 10)

		for sideIdx, orders := range [][]kraken.Level3Order{level3Data.Bids, level3Data.Asks} {
			side := "bid"

			if sideIdx == 1 {
				side = "ask"
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
					sequence,
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

				priceUnit, priceTime := data.GetDimension("spot:level3", "limit_price")
				qtyUnit, qtyTime := data.GetDimension("spot:level3", "order_qty")

				measurement.Write(
					data.NewExactMetric(
						"limit_price",
						order.LimitPrice,
						priceUnit,
						priceTime,
					),
					data.NewExactMetric(
						"order_qty",
						order.OrderQty,
						qtyUnit,
						qtyTime,
					),
				)

				storeTee.Push(measurement)
				sequence++
			}
		}
	}

	return nil
}

/*
handleTrade decodes a Kraken trade frame, updates price, stamps each trade with
its ingress sequence number and the book's executable touch at that number,
steps the workspace pipeline, and pushes the trade measurement to the store tee.
The touch travels on the trade, so every consumer reads the book as it stood at
the trade's SeqIdx rather than whatever the live book holds when a pipeline
consumer gets to it.
*/
func handleTrade(
	buf []byte,
	epoch int64,
	price *broker.Price,
	book *broker.Book,
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

		sequence, touchBid, touchAsk := book.Stamp(tradeItem.Symbol)

		measurement := data.NewMeasurement(
			epoch,
			tradeItem.Symbol,
			"spot:trade",
			sequence,
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

		priceUnit, priceTime := data.GetDimension("spot:trade", "price")
		qtyUnit, qtyTime := data.GetDimension("spot:trade", "qty")

		metrics := []*data.Metric{
			data.NewExactMetric(
				"price",
				&tradeItem.Price,
				priceUnit,
				priceTime,
			),
			data.NewMetric(
				"qty",
				tradeItem.Qty,
				qtyUnit,
				qtyTime,
			),
		}

		if touchBid != nil {
			bidUnit, bidTime := data.GetDimension("spot:trade", "touch_bid")
			askUnit, askTime := data.GetDimension("spot:trade", "touch_ask")

			metrics = append(metrics,
				data.NewExactMetric("touch_bid", touchBid, bidUnit, bidTime),
				data.NewExactMetric("touch_ask", touchAsk, askUnit, askTime),
			)
		}

		measurement.Write(metrics...)

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
