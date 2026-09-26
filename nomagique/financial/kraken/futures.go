package kraken

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/theapemachine/errnie"
)

/*
FuturesServer reads Kraken Futures frames into futures_ticker and futures_trade
records.
*/
type FuturesServer struct {
	records [][]byte
}

func NewFutures() *FuturesServer {
	return &FuturesServer{}
}

/* tickerFields maps the venue's ticker names onto the record's. */
var tickerFields = [][2]string{
	{"last", "last"}, {"bid", "bid"}, {"ask", "ask"}, {"bid_size", "bid_qty"},
	{"ask_size", "ask_qty"}, {"index", "index"}, {"markPrice", "mark"},
	{"openInterest", "openInterest"},
}

/* tradeFields maps the venue's trade names onto the record's. */
var tradeFields = [][2]string{
	{"price", "price"}, {"qty", "qty"}, {"side", "side"}, {"type", "type"},
}

func (server *FuturesServer) Write(ctx context.Context, call Futures_write) error {
	server.records = server.records[:0]
	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken.futures: failed to read data", err))
	}

	var frame map[string]any

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&frame); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken.futures: frame is not JSON", err))
	}

	// Subscription replies, alerts and info name an event; only data frames
	// carry readings.
	if _, control := frame["event"]; control {
		return nil
	}

	feed, _ := frame["feed"].(string)
	product, _ := frame["product_id"].(string)

	switch feed {
	case "ticker", "ticker_lite":
		return server.record(frame, "futures_ticker", product, frame, tickerFields, 0)
	case "trade":
		return server.record(frame, "futures_trade", product, frame, tradeFields, 0)
	case "trade_snapshot":
		trades, _ := frame["trades"].([]any)

		for index := len(trades) - 1; index >= 0; index-- {
			trade, ok := trades[index].(map[string]any)

			if !ok {
				return errnie.Error(errnie.Err(errnie.Validation, "kraken.futures: a snapshot trade is not an object", nil))
			}

			if err := server.record(frame, "futures_trade", product, trade, tradeFields, index); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
record adds one record on channel from source, carrying the frame's capture.
*/
func (server *FuturesServer) record(
	frame map[string]any, channel, product string, source map[string]any, fields [][2]string, index int,
) error {
	data := map[string]any{"symbol": product}

	for _, field := range fields {
		if value, found := source[field[0]]; found {
			data[field[1]] = value
		}
	}

	if at, found := source["time"].(json.Number); found {
		millis, err := at.Int64()
		if err != nil {
			return errnie.Error(err)
		}
		data["timestamp"] = time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
	}

	record := map[string]any{"channel": channel, "data": data}

	if capture, found := frame["capture"].(map[string]any); found {
		capture["record"] = index
		record["capture"] = capture
	}

	encoded, err := json.Marshal(record)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "kraken.futures: failed to encode a record", err))
	}

	server.records = append(server.records, encoded)
	return nil
}

func (server *FuturesServer) Done(ctx context.Context, call Futures_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "kraken.futures: failed to allocate results", err))
	}

	defer func() {
		server.records = server.records[:0]
	}()

	if len(server.records) == 0 {
		results.SetIdle()
		return nil
	}

	list, err := results.NewRecords(int32(len(server.records)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "kraken.futures: failed to allocate records", err))
	}

	for index, record := range server.records {
		if err := list.Set(index, record); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "kraken.futures: failed to set a record", err))
		}
	}

	return nil
}
