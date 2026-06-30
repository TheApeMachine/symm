package optimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/public"
)

type HistoricalOptions struct {
	Lookback time.Duration
	Interval int
	Client   *http.Client
	MaxPairs int
}

type historicalCandle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Count  float64
}

type historicalTrade struct {
	Time   time.Time
	Price  float64
	Qty    float64
	Side   string
	Symbol string
}

func BuildHistoricalReplay(ctx context.Context, symbols []string, options HistoricalOptions) ([]ReplayFrame, error) {
	options = normalizeHistoricalOptions(options)
	symbols = normalizeSymbols(symbols)
	if options.MaxPairs > 0 && len(symbols) > options.MaxPairs {
		symbols = symbols[:options.MaxPairs]
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("optimizer: no symbols for historical replay")
	}

	type result struct {
		symbol  string
		candles []historicalCandle
		trades  []historicalTrade
		err     error
	}

	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	start := time.Now().UTC().Add(-options.Lookback)
	jobs := make(chan string)
	results := make(chan result, len(symbols))
	workers := 6
	if workers > len(symbols) {
		workers = len(symbols)
	}

	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			for symbol := range jobs {
				candles, candleErr := fetchCandles(ctx, client, symbol, start, options.Interval)
				trades, tradeErr := fetchTrades(ctx, client, symbol, start)
				err := candleErr
				if err == nil && tradeErr != nil {
					err = tradeErr
				}
				results <- result{
					symbol:  symbol,
					candles: candles,
					trades:  trades,
					err:     err,
				}
			}
		})
	}

	for _, symbol := range symbols {
		jobs <- symbol
	}
	close(jobs)
	wait.Wait()
	close(results)

	builder := newFrameBuilder()
	var firstErr error
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
		for _, candle := range result.candles {
			builder.addCandle(result.symbol, candle)
		}
		for _, trade := range result.trades {
			builder.addTrade(trade)
		}
	}

	frames := builder.frames()
	if len(frames) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("optimizer: Kraken historical replay returned no frames")
	}

	return frames, nil
}

func normalizeHistoricalOptions(options HistoricalOptions) HistoricalOptions {
	if options.Lookback <= 0 {
		options.Lookback = 6 * time.Hour
	}
	if options.Interval <= 0 {
		options.Interval = 1
	}
	if options.MaxPairs <= 0 {
		options.MaxPairs = 16
	}

	return options
}

func normalizeSymbols(symbols []string) []string {
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || !strings.Contains(symbol, "/") {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	sort.Strings(out)

	return out
}

func fetchCandles(
	ctx context.Context,
	client *http.Client,
	symbol string,
	start time.Time,
	interval int,
) ([]historicalCandle, error) {
	var lastErr error
	for _, pair := range krakenPairCandidates(symbol) {
		candles, err := fetchCandlePair(ctx, client, pair, start, interval)
		if err == nil && len(candles) > 0 {
			return candles, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("optimizer: no OHLC data for %s", symbol)
}

func fetchCandlePair(
	ctx context.Context,
	client *http.Client,
	pair string,
	start time.Time,
	interval int,
) ([]historicalCandle, error) {
	values := url.Values{}
	values.Set("pair", pair)
	values.Set("interval", strconv.Itoa(interval))
	values.Set("since", strconv.FormatInt(start.Unix(), 10))

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		string(public.EndpointTypeOHLC)+"?"+values.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("optimizer: OHLC %s returned %s", pair, response.Status)
	}

	var payload struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Error) > 0 {
		return nil, fmt.Errorf("optimizer: OHLC %s: %s", pair, strings.Join(payload.Error, "; "))
	}

	candles := make([]historicalCandle, 0)
	for key, raw := range payload.Result {
		if key == "last" {
			continue
		}
		var rows [][]any
		if err := json.Unmarshal(raw, &rows); err != nil {
			continue
		}
		for _, row := range rows {
			if candle, ok := candleFromKraken(row); ok {
				candles = append(candles, candle)
			}
		}
	}
	sort.Slice(candles, func(first, second int) bool {
		return candles[first].Time.Before(candles[second].Time)
	})

	return candles, nil
}

func fetchTrades(
	ctx context.Context,
	client *http.Client,
	symbol string,
	start time.Time,
) ([]historicalTrade, error) {
	var lastErr error
	for _, pair := range krakenPairCandidates(symbol) {
		trades, err := fetchTradePair(ctx, client, pair, symbol, start)
		if err == nil && len(trades) > 0 {
			return trades, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}

	return nil, nil
}

func fetchTradePair(
	ctx context.Context,
	client *http.Client,
	pair string,
	symbol string,
	start time.Time,
) ([]historicalTrade, error) {
	values := url.Values{}
	values.Set("pair", pair)
	values.Set("since", strconv.FormatInt(start.UnixNano(), 10))

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		string(public.EndpointTypeTrades)+"?"+values.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("optimizer: Trades %s returned %s", pair, response.Status)
	}

	var payload struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Error) > 0 {
		return nil, fmt.Errorf("optimizer: Trades %s: %s", pair, strings.Join(payload.Error, "; "))
	}

	trades := make([]historicalTrade, 0)
	for key, raw := range payload.Result {
		if key == "last" {
			continue
		}
		var rows [][]any
		if err := json.Unmarshal(raw, &rows); err != nil {
			continue
		}
		for _, row := range rows {
			if trade, ok := tradeFromKraken(symbol, row); ok {
				trades = append(trades, trade)
			}
		}
	}
	sort.Slice(trades, func(first, second int) bool {
		return trades[first].Time.Before(trades[second].Time)
	})

	return trades, nil
}

func candleFromKraken(row []any) (historicalCandle, bool) {
	if len(row) < 7 {
		return historicalCandle{}, false
	}
	stamp, ok := numericAny(row[0])
	if !ok {
		return historicalCandle{}, false
	}
	open, ok := numericAny(row[1])
	if !ok {
		return historicalCandle{}, false
	}
	high, ok := numericAny(row[2])
	if !ok {
		return historicalCandle{}, false
	}
	low, ok := numericAny(row[3])
	if !ok {
		return historicalCandle{}, false
	}
	closePrice, ok := numericAny(row[4])
	if !ok || closePrice <= 0 {
		return historicalCandle{}, false
	}
	volume, _ := numericAny(row[6])
	count := 0.0
	if len(row) > 7 {
		count, _ = numericAny(row[7])
	}

	return historicalCandle{
		Time:   time.Unix(int64(stamp), 0).UTC(),
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePrice,
		Volume: volume,
		Count:  count,
	}, true
}

func tradeFromKraken(symbol string, row []any) (historicalTrade, bool) {
	if len(row) < 4 {
		return historicalTrade{}, false
	}
	price, ok := numericAny(row[0])
	if !ok || price <= 0 {
		return historicalTrade{}, false
	}
	qty, ok := numericAny(row[1])
	if !ok || qty <= 0 {
		return historicalTrade{}, false
	}
	stamp, ok := numericAny(row[2])
	if !ok {
		return historicalTrade{}, false
	}
	side := strings.ToLower(strings.TrimSpace(fmt.Sprint(row[3])))
	switch side {
	case "b", "buy":
		side = "buy"
	case "s", "sell":
		side = "sell"
	default:
		side = ""
	}

	return historicalTrade{
		Time:   time.Unix(0, int64(stamp*1e9)).UTC(),
		Price:  price,
		Qty:    qty,
		Side:   side,
		Symbol: symbol,
	}, true
}

func numericAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func krakenPairCandidates(symbol string) []string {
	clean := strings.ToUpper(strings.TrimSpace(symbol))
	compact := strings.ReplaceAll(clean, "/", "")
	candidates := []string{clean, compact}
	if strings.HasPrefix(compact, "BTC") {
		candidates = append(candidates, "XBT"+strings.TrimPrefix(compact, "BTC"))
	}
	if strings.Contains(clean, "/BTC") {
		candidates = append(candidates, strings.ReplaceAll(strings.Replace(clean, "/BTC", "/XBT", 1), "/", ""))
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	return out
}

type frameBuilder struct {
	byStamp map[int64]*ReplayFrame
}

func newFrameBuilder() *frameBuilder {
	return &frameBuilder{byStamp: make(map[int64]*ReplayFrame)}
}

func (builder *frameBuilder) frameAt(stamp time.Time) *ReplayFrame {
	key := stamp.UnixNano()
	frame := builder.byStamp[key]
	if frame == nil {
		frame = &ReplayFrame{
			Time:   stamp,
			Prices: make(map[string]float64),
		}
		builder.byStamp[key] = frame
	}

	return frame
}

func (builder *frameBuilder) addCandle(symbol string, candle historicalCandle) {
	frame := builder.frameAt(candle.Time)
	change := candle.Close - candle.Open
	changePct := 0.0
	if candle.Open > 0 {
		changePct = change / candle.Open * 100
	}
	frame.Prices[symbol] = candle.Close
	frame.Artifacts = append(frame.Artifacts, ReplayArtifact{
		Origin:    "kraken:public",
		Role:      "ticker",
		Type:      "update",
		Timestamp: candle.Time.UnixNano(),
		Payload: map[string]any{
			"channel": "ticker",
			"type":    "update",
			"data": []map[string]any{{
				"symbol":     symbol,
				"last":       candle.Close,
				"bid":        candle.Close,
				"ask":        candle.Close,
				"volume":     candle.Volume,
				"change":     change,
				"change_pct": changePct,
			}},
		},
	})
	frame.Artifacts = append(frame.Artifacts, ReplayArtifact{
		Origin:    "kraken:public",
		Role:      "ohlc",
		Type:      "update",
		Timestamp: candle.Time.UnixNano(),
		Payload: map[string]any{
			"channel": "ohlc",
			"type":    "update",
			"data": []map[string]any{{
				"symbol": symbol,
				"open":   candle.Open,
				"high":   candle.High,
				"low":    candle.Low,
				"close":  candle.Close,
				"volume": candle.Volume,
				"count":  candle.Count,
			}},
		},
	})
}

func (builder *frameBuilder) addTrade(trade historicalTrade) {
	stamp := trade.Time.Truncate(time.Second)
	frame := builder.frameAt(stamp)
	frame.Prices[trade.Symbol] = trade.Price
	frame.Artifacts = append(frame.Artifacts, ReplayArtifact{
		Origin:    "kraken:public",
		Role:      "trade",
		Type:      "update",
		Timestamp: stamp.UnixNano(),
		Payload: map[string]any{
			"channel": "trade",
			"type":    "update",
			"data": []map[string]any{{
				"symbol": trade.Symbol,
				"side":   trade.Side,
				"price":  trade.Price,
				"qty":    trade.Qty,
			}},
		},
	})
}

func (builder *frameBuilder) frames() []ReplayFrame {
	keys := make([]int64, 0, len(builder.byStamp))
	for key := range builder.byStamp {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(first, second int) bool {
		return keys[first] < keys[second]
	})

	frames := make([]ReplayFrame, 0, len(keys))
	for _, key := range keys {
		frame := builder.byStamp[key]
		frames = append(frames, *frame)
	}

	return frames
}
