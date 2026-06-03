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
	testSymbolPrimary          = "SYN/EUR"
	testSymbolSecondary        = "LAG/EUR"
	testSymbolLeader           = "LEAD/EUR"
	correlationWarmupBatches   = 33
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

	line, err := sonic.Marshal(public.SocketMessage{
		Channel: channel,
		Type:    frameType,
		Data:    raw,
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
	builder.AppendTickerAt(symbol, last, bid, ask, changePct, builder.timestamp())
}

func (builder *CaptureBuilder) AppendTickerAt(
	symbol string, last, bid, ask, changePct float64, at time.Time,
) {
	builder.appendFrame(public.TickerChannel, "update", []market.TickerUpdate{{
		Symbol:    symbol,
		Last:      last,
		Bid:       bid,
		Ask:       ask,
		BidQty:    10,
		AskQty:    10,
		ChangePct: changePct,
		Volume:    1000,
		Timestamp: at.UTC().Format(time.RFC3339Nano),
	}})
}

func (builder *CaptureBuilder) AppendBookSnapshot(
	symbol string, bidPrice, bidQty, askPrice, askQty float64,
) {
	book := market.Book{
		Symbol:    symbol,
		Bids:      []market.BookLevel{{Price: bidPrice, Qty: bidQty}},
		Asks:      []market.BookLevel{{Price: askPrice, Qty: askQty}},
		Timestamp: builder.timestampRFC3339(),
	}
	book.SetEnvelopeType(market.BookSnapshot)

	builder.appendFrame(public.BookChannel, market.BookSnapshot, []market.Book{book})
}

func (builder *CaptureBuilder) AppendBuyTrades(symbol string, count int, startPrice, qty float64) {
	trades := make([]market.TradeUpdate, 0, count)

	for index := range count {
		trades = append(trades, market.TradeUpdate{
			Symbol:    symbol,
			Side:      "buy",
			Price:     startPrice + float64(index)*0.01,
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
			Symbol: symbol,
			Bids:   []market.BookLevel{{Price: 100, Qty: depth}},
			Asks:   []market.BookLevel{{Price: askPrice, Qty: depth * 0.5}},
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
	start := time.Now().UTC().Add(-90 * time.Minute)

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

func leadLagPostReplayTickers() []market.TickerUpdate {
	origin := time.Now().UTC().Add(-75 * time.Minute)
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
	builder.AppendTicker(testSymbolPrimary, 55, 50, 60, -12)
	builder.AppendTicker(testSymbolLeader, 70, 69, 71, -25)
	builder.AppendTicker(testSymbolSecondary, 40, 39, 41, -50)
	builder.AppendBookSnapshot(testSymbolPrimary, 50, 1, 60, 1)
	builder.AppendSellTrade(testSymbolPrimary, 52, 25)
	builder.AppendTicker(testSymbolPrimary, 58, 54, 62, -11)
	builder.AppendBookSnapshot(testSymbolPrimary, 54, 8, 62, 8)
	builder.AppendSentimentSlumpCrossSection()
}
