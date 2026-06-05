package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Candle is one OHLC candle from GET /public/OHLC.
Wire row: [time, open, high, low, close, vwap, volume, count].
See https://docs.kraken.com/api/docs/rest-api/get-ohlc-data
*/
type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	VWAP   float64
	Volume float64
	Count  int64
}

/*
OHLC is the /public/OHLC result: candles keyed by Kraken pair name plus the
pagination cursor (Last is an integer second-since-epoch).

Candlestick bars compress every trade in a fixed interval into open, high, low,
and close, plus the interval's volume-weighted average price and trade count. It
is the standard summary of price action and participation over time -- VWAP gives
the fair transacted price -- served historically and paginated far beyond what
live feeds retain.
*/
type OHLC struct {
	Last    int64
	Candles map[string][]Candle
}

var candleFloatFields = []struct {
	name  string
	index int
}{
	{name: "open", index: 1},
	{name: "high", index: 2},
	{name: "low", index: 3},
	{name: "close", index: 4},
	{name: "vwap", index: 5},
	{name: "volume", index: 6},
}

func (ohlc *OHLC) UnmarshalJSON(data []byte) error {
	raw := make(map[string]json.RawMessage)

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	ohlc.Candles = make(map[string][]Candle, len(raw))

	if lastRaw, ok := raw["last"]; ok {
		last, err := jsonInt64(lastRaw)

		if err != nil {
			return fmt.Errorf("ohlc last: %w", err)
		}

		ohlc.Last = last
		delete(raw, "last")
	}

	for pair, rowsRaw := range raw {
		var rows [][]json.RawMessage

		if err := json.Unmarshal(rowsRaw, &rows); err != nil {
			return fmt.Errorf("ohlc %s rows: %w", pair, err)
		}

		candles := make([]Candle, 0, len(rows))

		for rowIndex, row := range rows {
			candle, err := candleFromRow(row)

			if err != nil {
				return fmt.Errorf("ohlc %s row %d: %w", pair, rowIndex, err)
			}

			candles = append(candles, candle)
		}

		ohlc.Candles[pair] = candles
	}

	return nil
}

func candleFromRow(row []json.RawMessage) (Candle, error) {
	if len(row) != 8 {
		return Candle{}, fmt.Errorf("expected 8 fields, got %d", len(row))
	}

	timestamp, err := jsonInt64(row[0])

	if err != nil {
		return Candle{}, fmt.Errorf("time: %w", err)
	}

	values, err := candleFloatValues(row)

	if err != nil {
		return Candle{}, err
	}

	count, err := jsonInt64(row[7])

	if err != nil {
		return Candle{}, fmt.Errorf("count: %w", err)
	}

	return Candle{
		Time:   time.Unix(timestamp, 0).UTC(),
		Open:   values[0],
		High:   values[1],
		Low:    values[2],
		Close:  values[3],
		VWAP:   values[4],
		Volume: values[5],
		Count:  count,
	}, nil
}

func candleFloatValues(row []json.RawMessage) ([6]float64, error) {
	var values [6]float64

	for fieldIndex, field := range candleFloatFields {
		value, err := jsonFloat(row[field.index])

		if err != nil {
			return values, fmt.Errorf("%s: %w", field.name, err)
		}

		values[fieldIndex] = value
	}

	return values, nil
}

func jsonFloat(raw json.RawMessage) (float64, error) {
	var number float64

	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}

	var text string

	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}

	return strconv.ParseFloat(text, 64)
}

func jsonInt64(raw json.RawMessage) (int64, error) {
	var number int64

	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}

	var text string

	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}

	return strconv.ParseInt(text, 10, 64)
}

/*
NewOHLC fetches OHLC data. intervalMinutes is the candle width in minutes.
Pass since 0 to omit the since parameter.
*/
func NewOHLC(
	ctx context.Context,
	client *public.Rest,
	pair string,
	intervalMinutes int,
	since int64,
) (*OHLC, error) {
	ohlc := &OHLC{}
	params := fiber.Map{
		"pair":     pair,
		"interval": intervalMinutes,
	}

	if since > 0 {
		params["since"] = since
	}

	return ohlc, errnie.Error(client.Get(ctx, params, ohlc))
}
