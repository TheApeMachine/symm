package integration

import (
	"bytes"
	"io"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	testSymbolPrimary        = "SYN/EUR"
	testSymbolSecondary      = "LAG/EUR"
	testSymbolLeader         = "LEAD/EUR"
	correlationWarmupBatches = 33
)

/*
CaptureBuilder writes deterministic Kraken replay JSONL lines.
*/
type CaptureBuilder struct {
	buffer bytes.Buffer
	origin time.Time
	step   time.Duration
	tick   int
}

func NewCaptureBuilder(origin time.Time) *CaptureBuilder {
	return &CaptureBuilder{
		origin: origin,
		step:   50 * time.Millisecond,
	}
}

func (builder *CaptureBuilder) Reader() io.Reader {
	return bytes.NewReader(builder.buffer.Bytes())
}

func (builder *CaptureBuilder) appendFrame(channel, frameType string, payload any) {
	builder.tick++

	raw, err := sonic.Marshal(payload)

	if err != nil {
		panic(err)
	}

	line, err := sonic.Marshal(map[string]any{
		"channel": channel,
		"type":    frameType,
		"data":    raw,
	})

	if err != nil {
		panic(err)
	}

	builder.buffer.Write(line)
	builder.buffer.WriteByte('\n')
}

func (builder *CaptureBuilder) timestamp() time.Time {
	return builder.origin.Add(time.Duration(builder.tick) * builder.step)
}

func (builder *CaptureBuilder) timestampRFC3339() string {
	return builder.timestamp().UTC().Format(time.RFC3339Nano)
}

func (builder *CaptureBuilder) Advance(duration time.Duration) {
	steps := int(duration / builder.step)

	if steps <= 0 {
		steps = 1
	}

	builder.tick += steps
}

func (builder *CaptureBuilder) AppendInstrumentCatalog() {
	builder.appendFrame(public.InstrumentsChannel, market.BookSnapshot, market.InstrumentUpdate{
		Pairs: []market.InstrumentPair{
			{
				Symbol:         testSymbolPrimary,
				Base:           "SYN",
				Quote:          "EUR",
				Status:         "online",
				QtyPrecision:   8,
				QtyIncrement:   0.0001,
				PricePrecision: 2,
				PriceIncrement: 0.01,
				QtyMin:         0.0001,
				CostMin:        1,
			},
			{
				Symbol:         testSymbolSecondary,
				Base:           "LAG",
				Quote:          "EUR",
				Status:         "online",
				QtyPrecision:   8,
				QtyIncrement:   0.0001,
				PricePrecision: 2,
				PriceIncrement: 0.01,
				QtyMin:         0.0001,
				CostMin:        1,
			},
			{
				Symbol:         testSymbolLeader,
				Base:           "LEAD",
				Quote:          "EUR",
				Status:         "online",
				QtyPrecision:   8,
				QtyIncrement:   0.0001,
				PricePrecision: 2,
				PriceIncrement: 0.01,
				QtyMin:         0.0001,
				CostMin:        1,
			},
		},
	})
}

func (builder *CaptureBuilder) AppendTicker(
	symbol string, last, bid, ask, changePct float64,
) {
	builder.AppendTickerAtVolume(symbol, last, bid, ask, changePct, 1000, builder.timestamp())
}

func (builder *CaptureBuilder) AppendTickerAt(
	symbol string, last, bid, ask, changePct float64, at time.Time,
) {
	builder.AppendTickerAtVolume(symbol, last, bid, ask, changePct, 1000, at)
}

func (builder *CaptureBuilder) AppendTickerVolume(
	symbol string, last, bid, ask, changePct, volume float64,
) {
	builder.AppendTickerAtVolume(symbol, last, bid, ask, changePct, volume, builder.timestamp())
}

func (builder *CaptureBuilder) AppendTickerAtVolume(
	symbol string, last, bid, ask, changePct, volume float64, at time.Time,
) {
	builder.appendFrame(public.TickerChannel, "update", []market.TickerUpdate{{
		Symbol:    symbol,
		Last:      last,
		Bid:       bid,
		Ask:       ask,
		BidQty:    10,
		AskQty:    10,
		ChangePct: changePct,
		Volume:    volume,
		Timestamp: at.UTC().Format(time.RFC3339Nano),
	}})
}

func (builder *CaptureBuilder) AppendTickerBatch(rows []market.TickerUpdate) {
	for rowIndex := range rows {
		if rows[rowIndex].Timestamp != "" {
			continue
		}

		rows[rowIndex].Timestamp = builder.timestampRFC3339()
	}

	builder.appendFrame(public.TickerChannel, "update", rows)
}

func (builder *CaptureBuilder) AppendBookSnapshot(
	symbol string, bidPrice, bidQty, askPrice, askQty float64,
) {
	builder.AppendBookSnapshotAt(
		symbol, bidPrice, bidQty, askPrice, askQty, builder.timestamp(),
	)
}

func (builder *CaptureBuilder) AppendBookSnapshotAt(
	symbol string,
	bidPrice float64,
	bidQty float64,
	askPrice float64,
	askQty float64,
	at time.Time,
) {
	builder.AppendBookSnapshotLevelsAt(
		symbol,
		[]market.BookLevel{{Price: bidPrice, Qty: bidQty}},
		[]market.BookLevel{{Price: askPrice, Qty: askQty}},
		at,
	)
}

func (builder *CaptureBuilder) AppendBookSnapshotLevels(
	symbol string,
	bids []market.BookLevel,
	asks []market.BookLevel,
) {
	builder.AppendBookSnapshotLevelsAt(symbol, bids, asks, builder.timestamp())
}

func (builder *CaptureBuilder) AppendBookSnapshotLevelsAt(
	symbol string,
	bids []market.BookLevel,
	asks []market.BookLevel,
	at time.Time,
) {
	book := market.Book{
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: at.UTC().Format(time.RFC3339Nano),
	}
	book.SetEnvelopeType(market.BookSnapshot)

	builder.appendFrame(public.BookChannel, market.BookSnapshot, []market.Book{book})
}

func (builder *CaptureBuilder) AppendTradeAt(
	symbol string,
	side string,
	price float64,
	qty float64,
	at time.Time,
) {
	builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       qty,
		OrdType:   "market",
		TradeID:   int64(builder.tick * 1000),
		Timestamp: at,
	}})
}

func (builder *CaptureBuilder) AppendBuyTrades(symbol string, count int, startPrice, qty float64) {
	builder.AppendBuyTradeRamp(symbol, count, startPrice, 0.01, qty)
}

func (builder *CaptureBuilder) AppendBuyTradeRamp(
	symbol string,
	count int,
	startPrice float64,
	priceStep float64,
	qty float64,
) {
	trades := make([]market.TradeUpdate, 0, count)

	for index := range count {
		trades = append(trades, market.TradeUpdate{
			Symbol:    symbol,
			Side:      "buy",
			Price:     startPrice + float64(index)*priceStep,
			Qty:       qty,
			OrdType:   "market",
			TradeID:   int64(builder.tick*1000 + index),
			Timestamp: builder.timestamp().Add(time.Duration(index) * time.Millisecond),
		})
	}

	builder.appendFrame(public.TradesChannel, "update", trades)
}

func (builder *CaptureBuilder) AppendSellTrade(symbol string, price, qty float64) {
	builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
		Symbol:    symbol,
		Side:      "sell",
		Price:     price,
		Qty:       qty,
		OrdType:   "market",
		TradeID:   int64(builder.tick * 1000),
		Timestamp: builder.timestamp(),
	}})
}

func (builder *CaptureBuilder) AppendSellTrades(symbol string, count int, startPrice, qty float64) {
	trades := make([]market.TradeUpdate, 0, count)

	for index := range count {
		trades = append(trades, market.TradeUpdate{
			Symbol:    symbol,
			Side:      "sell",
			Price:     startPrice - float64(index)*0.01,
			Qty:       qty,
			OrdType:   "market",
			TradeID:   int64(builder.tick*1000 + index),
			Timestamp: builder.timestamp().Add(time.Duration(index) * time.Millisecond),
		})
	}

	builder.appendFrame(public.TradesChannel, "update", trades)
}

func (builder *CaptureBuilder) AppendBaselineMarket() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0.5)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 20, 100.5, 20)
	builder.AppendBuyTrades(testSymbolPrimary, 32, 100, 2)
}

func (builder *CaptureBuilder) AppendSentimentSlumpCrossSection() {
	builder.AppendTicker(testSymbolLeader, 100, 99, 101, 4)
	builder.AppendTicker(testSymbolSecondary, 80, 79, 81, -3)
	builder.AppendTicker(testSymbolPrimary, 70, 69, 71, -2.5)
}

func (builder *CaptureBuilder) AppendTradeBurst(
	symbol string, count int, startPrice, qty float64, side string,
) {
	trades := make([]market.TradeUpdate, 0, count)

	for index := range count {
		tradeSide := side

		if side == "alternate" {
			if index%2 == 0 {
				tradeSide = "buy"
			} else {
				tradeSide = "sell"
			}
		}

		trades = append(trades, market.TradeUpdate{
			Symbol:    symbol,
			Side:      tradeSide,
			Price:     startPrice + float64(index)*0.01,
			Qty:       qty + float64(index%5)*0.1,
			OrdType:   "market",
			TradeID:   int64(builder.tick*10000 + index),
			Timestamp: builder.timestamp().Add(time.Duration(index) * 100 * time.Millisecond),
		})
	}

	builder.appendFrame(public.TradesChannel, "update", trades)
}

func (builder *CaptureBuilder) AppendBookThinning(symbol string, frames int) {
	builder.AppendTicker(symbol, 100, 99, 101, 0)
	for index := range frames {
		depth := 20.0 - float64(index)*2
		askPrice := 101.0 + float64(index)*0.5

		book := market.Book{
			Symbol:    symbol,
			Bids:      []market.BookLevel{{Price: 100, Qty: depth}},
			Asks:      []market.BookLevel{{Price: askPrice, Qty: depth * 0.5}},
			Timestamp: builder.timestampRFC3339(),
		}
		book.SetEnvelopeType(market.BookSnapshot)

		builder.appendFrame(public.BookChannel, market.BookSnapshot, []market.Book{book})
	}
}

func (builder *CaptureBuilder) AppendBookLevelShrink(
	symbol string, bidPrice, prevQty, nextQty float64,
) {
	book := market.Book{
		Symbol:    symbol,
		Bids:      []market.BookLevel{{Price: bidPrice, Qty: nextQty}},
		Asks:      []market.BookLevel{{Price: bidPrice + 2, Qty: 10}},
		Timestamp: builder.timestampRFC3339(),
	}
	book.SetEnvelopeType(market.BookUpdate)

	builder.appendFrame(public.BookChannel, market.BookUpdate, []market.Book{book})
}

func (builder *CaptureBuilder) AppendToxicityCancelWall(symbol string, mid float64) {
	builder.AppendTicker(symbol, mid, mid-0.5, mid+0.5, 0)
	builder.AppendBookSnapshot(symbol, mid-0.5, 100, mid+0.5, 10)
	builder.AppendBookLevelShrink(symbol, mid-0.5, 100, 5)
	builder.AppendBookLevelShrink(symbol, mid-0.5, 5, 0)
}

/*
AppendLevel3NearTouchChurn emits per-order add/delete churn on the level3 channel so
integration harnesses can exercise the toxicity L3 path before live credentials exist.
*/
func (builder *CaptureBuilder) AppendLevel3NearTouchChurn(symbol string, price float64) {
	timestamp := builder.timestampRFC3339()

	builder.appendFrame(public.Level3Channel, "update", []map[string]any{{
		"symbol": symbol,
		"bids": []map[string]any{
			{
				"event":       "add",
				"order_id":    "l3-churn",
				"limit_price": price,
				"order_qty":   100,
				"timestamp":   timestamp,
			},
			{
				"event":       "delete",
				"order_id":    "l3-churn",
				"limit_price": price,
				"order_qty":   100,
				"timestamp":   timestamp,
			},
		},
		"asks": []map[string]any{},
	}})
}

func (builder *CaptureBuilder) AppendLiquidityCrossSection() {
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 1)
	builder.AppendTicker(testSymbolSecondary, 8, 7.9, 8.1, 0.5)
	builder.AppendTicker(testSymbolLeader, 12, 11.9, 12.1, 2)
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 1)
	builder.AppendTicker(testSymbolSecondary, 8, 7.9, 8.1, 0.5)
	builder.AppendTicker(testSymbolLeader, 12, 11.9, 12.1, 2)
}

func (builder *CaptureBuilder) AppendPumpLift(symbol string, count int) {
	for index := range count {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    symbol,
			Side:      "buy",
			Price:     10 + float64(index)*0.05,
			Qty:       1.5 + float64(index)*0.25,
			OrdType:   "market",
			TradeID:   int64(builder.tick*1000 + index),
			Timestamp: builder.timestamp().Add(time.Duration(index) * time.Millisecond),
		}})
	}
}

func (builder *CaptureBuilder) AppendDepthflowTape(symbol string) {
	builder.AppendTicker(symbol, 100, 99, 101, 0.5)
	builder.AppendBookSnapshot(symbol, 99, 8, 101, 4)
	builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
		Symbol: symbol, Side: "buy", Price: 100, Qty: 3,
		Timestamp: builder.timestamp(),
	}})
}

func (builder *CaptureBuilder) AppendCausalCrossSection() {
	for _, symbol := range []string{testSymbolPrimary, testSymbolSecondary, testSymbolLeader} {
		builder.AppendTicker(symbol, 100, 99, 101, 1)
		builder.AppendBookSnapshot(symbol, 99, 8, 101, 6)
		builder.AppendBuyTrades(symbol, 12, 100, 2)
	}
}

func (builder *CaptureBuilder) AppendLeadLagStall() {
	start := builder.origin.Add(-90 * time.Minute)

	for index := range 16 {
		at := start.Add(time.Duration(index) * 5 * time.Minute)
		builder.AppendTickerAt(testSymbolLeader, 100, 99, 101, 0, at)
		builder.AppendTickerAt(testSymbolPrimary, 100.01, 99, 101, 0.5, at)
	}
}

func correlationPostReplayTradeBatches() [][]market.TradeUpdate {
	prices := map[string]float64{
		testSymbolPrimary:   100,
		testSymbolSecondary: 50,
		testSymbolLeader:    75,
	}
	batches := make([][]market.TradeUpdate, 0, correlationWarmupBatches)

	for range correlationWarmupBatches {
		batch := make([]market.TradeUpdate, 0, len(prices))

		for symbol, price := range prices {
			batch = append(batch, market.TradeUpdate{
				Symbol: symbol,
				Side:   "buy",
				Price:  price,
				Qty:    1,
			})
			prices[symbol] = price * 1.02
		}

		batches = append(batches, batch)
	}

	return batches
}

func causalNoisePostReplayTickers() []market.TickerUpdate {
	origin := causalNoisePostOrigin()

	return []market.TickerUpdate{{
		Symbol:    testSymbolPrimary,
		Last:      100,
		Bid:       99.5,
		Ask:       100.5,
		ChangePct: 0,
		Volume:    1000,
		Timestamp: origin.UTC().Format(time.RFC3339Nano),
	}}
}

func causalNoisePostReplayTrades() []market.TradeUpdate {
	origin := causalNoisePostOrigin()
	trades := make([]market.TradeUpdate, 0, 8)

	for index := range 8 {
		trades = append(trades, market.TradeUpdate{
			Symbol:    testSymbolPrimary,
			Side:      "buy",
			Price:     100,
			Qty:       2,
			OrdType:   "market",
			TradeID:   int64(90_000 + index),
			Timestamp: origin.Add(time.Duration(index+1) * 600 * time.Millisecond),
		})
	}

	return trades
}

func causalNoisePostOrigin() time.Time {
	return time.Date(2026, 6, 3, 12, 20, 0, 0, time.UTC)
}

func leadLagInefficientLagTickers() []market.TickerUpdate {
	origin := leadLagPostOrigin()
	anchor := leadLagPatternPrices(18, 100, 0.45)
	tickers := make([]market.TickerUpdate, 0, len(anchor)*2)
	lagBars := 5

	for index := range anchor {
		at := origin.Add(time.Duration(index) * 5 * time.Minute)
		followerIndex := index - lagBars
		follower := anchor[0]

		if followerIndex >= 0 {
			follower = anchor[followerIndex]
		}

		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolLeader, Last: anchor[index], Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolPrimary, Last: follower, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
	}

	finalAt := origin.Add(time.Duration(len(anchor)) * 5 * time.Minute)
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolLeader, Last: anchor[len(anchor)-1] + 8, Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolPrimary, Last: anchor[len(anchor)-lagBars], Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})

	return tickers
}

func leadLagSynchronizedDriftTickers() []market.TickerUpdate {
	origin := leadLagPostOrigin()
	anchor := leadLagPatternPrices(18, 100, 0.35)
	tickers := make([]market.TickerUpdate, 0, len(anchor)*2)

	for index := range anchor {
		at := origin.Add(time.Duration(index) * 5 * time.Minute)
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolLeader, Last: anchor[index], Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolPrimary, Last: anchor[index] + 0.02, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
	}

	finalAt := origin.Add(time.Duration(len(anchor)) * 5 * time.Minute)
	finalPrice := anchor[len(anchor)-1] + 8
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolLeader, Last: finalPrice, Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolPrimary, Last: finalPrice + 0.02, Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})

	return tickers
}

func leadLagDecoupledMoveTickers() []market.TickerUpdate {
	origin := leadLagPostOrigin()
	tickers := make([]market.TickerUpdate, 0, 38)

	for index := range 18 {
		at := origin.Add(time.Duration(index) * 5 * time.Minute)
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolLeader, Last: 100 + float64(index)*0.3, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolPrimary, Last: 100 - float64(index)*0.35, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
	}

	finalAt := origin.Add(18 * 5 * time.Minute)
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolLeader, Last: 116, Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})
	tickers = append(tickers, market.TickerUpdate{
		Symbol: testSymbolPrimary, Last: 93.5, Bid: 99, Ask: 101,
		Timestamp: finalAt.UTC().Format(time.RFC3339Nano),
	})

	return tickers
}

func correlationDecoupledTradeBatches() [][]market.TradeUpdate {
	prices := map[string]float64{
		testSymbolPrimary:   100,
		testSymbolSecondary: 50,
		testSymbolLeader:    75,
	}
	primaryMoves := []float64{1.025, 0.992, 1.018, 0.985, 1.012, 1.006, 0.996, 1.021}
	batches := make([][]market.TradeUpdate, 0, correlationWarmupBatches)

	for batchIndex := range correlationWarmupBatches {
		batch := make([]market.TradeUpdate, 0, len(prices))

		for symbol, price := range prices {
			side := "buy"

			batch = append(batch, market.TradeUpdate{
				Symbol: symbol,
				Side:   side,
				Price:  price,
				Qty:    1,
			})

			if symbol == testSymbolPrimary {
				prices[symbol] = price * primaryMoves[batchIndex%len(primaryMoves)]
			} else {
				prices[symbol] = price * 1.02
			}
		}

		batches = append(batches, batch)
	}

	return batches
}

func correlationNoiseTradeBatches() [][]market.TradeUpdate {
	prices := map[string]float64{
		testSymbolPrimary:   100,
		testSymbolSecondary: 50,
		testSymbolLeader:    75,
	}
	moves := map[string][]float64{
		testSymbolPrimary:   {1.0001, 0.9999, 1.0001, 0.9999},
		testSymbolSecondary: {1.0120, 0.9910, 1.0100, 0.9920},
		testSymbolLeader:    {1.0110, 0.9900, 1.0090, 0.9910},
	}
	batches := make([][]market.TradeUpdate, 0, correlationWarmupBatches)

	for batchIndex := range correlationWarmupBatches {
		batch := make([]market.TradeUpdate, 0, len(prices))

		for symbol, price := range prices {
			batch = append(batch, market.TradeUpdate{
				Symbol: symbol,
				Side:   "buy",
				Price:  price,
				Qty:    0.01,
			})
			series := moves[symbol]
			prices[symbol] = price * series[batchIndex%len(series)]
		}

		batches = append(batches, batch)
	}

	return batches
}

func correlationDivergentStressTradeBatches() [][]market.TradeUpdate {
	prices := map[string]float64{
		testSymbolPrimary:   100,
		testSymbolSecondary: 50,
		testSymbolLeader:    75,
	}
	batches := make([][]market.TradeUpdate, 0, correlationWarmupBatches)

	for range correlationWarmupBatches {
		batch := make([]market.TradeUpdate, 0, len(prices))

		for symbol, price := range prices {
			side := "buy"
			next := price * 1.02

			if symbol == testSymbolPrimary {
				side = "sell"
				next = price * 0.97
			}

			batch = append(batch, market.TradeUpdate{
				Symbol: symbol,
				Side:   side,
				Price:  price,
				Qty:    1,
			})
			prices[symbol] = next
		}

		batches = append(batches, batch)
	}

	return batches
}

func leadLagPostReplayTickers() []market.TickerUpdate {
	origin := leadLagPostOrigin()
	tickers := make([]market.TickerUpdate, 0, 28)

	for index := range 14 {
		at := origin.Add(time.Duration(index) * 5 * time.Minute)
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolLeader, Last: 100, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
		tickers = append(tickers, market.TickerUpdate{
			Symbol: testSymbolPrimary, Last: 100.01, Bid: 99, Ask: 101,
			Timestamp: at.UTC().Format(time.RFC3339Nano),
		})
	}

	return tickers
}

func leadLagPostOrigin() time.Time {
	return time.Date(2026, 6, 3, 11, 50, 0, 0, time.UTC)
}

func leadLagPatternPrices(count int, start float64, scale float64) []float64 {
	moves := []float64{1, -0.4, 1.3, -0.7, 0.9, -0.2, 1.1, -0.6}
	prices := make([]float64, count)
	price := start

	for index := range count {
		if index > 0 {
			price += moves[index%len(moves)] * scale
		}

		prices[index] = price
	}

	return prices
}

func (builder *CaptureBuilder) AppendCorrelationHerd() {
	prices := map[string]float64{
		testSymbolPrimary:   100,
		testSymbolSecondary: 50,
		testSymbolLeader:    75,
	}

	for range 40 {
		for symbol, price := range prices {
			builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
				Symbol:    symbol,
				Side:      "buy",
				Price:     price,
				Qty:       1,
				Timestamp: builder.timestamp(),
			}})
			prices[symbol] = price * 1.01
		}

		builder.tick += 6
	}
}

func (builder *CaptureBuilder) AppendBlackSwanCrash() {
	builder.AppendTicker(testSymbolLeader, 100, 99, 101, -1)
	builder.AppendTicker(testSymbolSecondary, 80, 79, 81, -4)
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, -0.5)
	builder.AppendBookSnapshot(testSymbolPrimary, 99, 5, 101, 5)
	builder.AppendBuyTrades(testSymbolPrimary, 24, 100, 1)
	builder.Advance(20 * time.Minute)
	builder.AppendTicker(testSymbolPrimary, 55, 50, 60, -12)
	builder.AppendTicker(testSymbolLeader, 70, 69, 71, -25)
	builder.AppendTicker(testSymbolSecondary, 40, 39, 41, -50)
	builder.AppendBookSnapshot(testSymbolPrimary, 50, 1, 60, 1)
	builder.AppendSellTrades(testSymbolPrimary, 8, 52, 25)
	builder.AppendTicker(testSymbolPrimary, 58, 54, 62, -11)
	builder.AppendBookSnapshot(testSymbolPrimary, 54, 8, 62, 8)
	builder.AppendSentimentSlumpCrossSection()
}
