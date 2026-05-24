package rest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/symm/kraken/core"
)

const ohlcPath = "/0/public/OHLC"

/*
Candle is one Kraken OHLC interval.
*/
type Candle struct {
	Time                   time.Time
	Open, High, Low, Close float64
	Volume                 float64
}

/*
FetchOHLC loads recent OHLC candles for one Kraken pair name.
*/
func FetchOHLC(pair string, intervalMinutes int) ([]Candle, error) {
	if strings.TrimSpace(pair) == "" {
		return nil, fmt.Errorf("pair is required")
	}

	if intervalMinutes <= 0 {
		return nil, fmt.Errorf("interval must be positive")
	}

	query := url.Values{}
	query.Set("pair", pair)
	query.Set("interval", strconv.Itoa(intervalMinutes))

	requestURL := strings.Join([]string{core.KRAKEN_API_URL, ohlcPath}, "") + "?" + query.Encode()

	response, err := client.Get(requestURL)

	if err != nil {
		return nil, err
	}

	defer response.Close()

	var payload ohlcResponse

	if err = json.Unmarshal(response.Body(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal ohlc: %w", err)
	}

	if len(payload.Error) > 0 {
		return nil, fmt.Errorf("kraken ohlc error: %v", payload.Error)
	}

	rows, err := payload.candleRows()

	if err != nil {
		return nil, err
	}

	return parseCandles(rows), nil
}

type ohlcResponse struct {
	Error  []string       `json:"error"`
	Result map[string]any `json:"result"`
}

func (response *ohlcResponse) candleRows() ([][]any, error) {
	if response == nil || len(response.Result) == 0 {
		return nil, fmt.Errorf("empty ohlc result")
	}

	for key, value := range response.Result {
		if key == "last" {
			continue
		}

		rows, err := asCandleRows(value)

		if err != nil {
			return nil, fmt.Errorf("unexpected ohlc rows for %q: %w", key, err)
		}

		return rows, nil
	}

	return nil, fmt.Errorf("ohlc rows missing from result")
}

func asCandleRows(value any) ([][]any, error) {
	switch typed := value.(type) {
	case []any:
		rows := make([][]any, 0, len(typed))

		for _, row := range typed {
			columns, ok := row.([]any)

			if !ok {
				return nil, fmt.Errorf("row is not an array")
			}

			rows = append(rows, columns)
		}

		return rows, nil
	case [][]any:
		return typed, nil
	default:
		return nil, fmt.Errorf("rows are not an array")
	}
}

func parseCandles(rows [][]any) []Candle {
	candles := make([]Candle, 0, len(rows))

	for _, row := range rows {
		candle, ok := parseCandle(row)

		if !ok {
			continue
		}

		candles = append(candles, candle)
	}

	return candles
}

func parseCandle(row []any) (Candle, bool) {
	if len(row) < 8 {
		return Candle{}, false
	}

	seconds, ok := numericValue(row[0])

	if !ok {
		return Candle{}, false
	}

	open, okOpen := numericValue(row[1])
	high, okHigh := numericValue(row[2])
	low, okLow := numericValue(row[3])
	closePx, okClose := numericValue(row[4])
	volume, okVolume := numericValue(row[6])

	if !okOpen || !okHigh || !okLow || !okClose || !okVolume {
		return Candle{}, false
	}

	return Candle{
		Time:   time.Unix(int64(seconds), 0).UTC(),
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePx,
		Volume: volume,
	}, true
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)

		return parsed, err == nil
	default:
		return 0, false
	}
}
