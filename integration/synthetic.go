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
	testSymbolPrimary   = "SYN/EUR"
	testSymbolSecondary = "LAG/EUR"
	testSymbolLeader    = "LEAD/EUR"
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
	builder.appendFrame(public.TickerChannel, "update", []market.TickerUpdate{{
		Symbol:    symbol,
		Last:      last,
		Bid:       bid,
		Ask:       ask,
		BidQty:    10,
		AskQty:    10,
		ChangePct: changePct,
		Volume:    1000,
		Timestamp: builder.timestampRFC3339(),
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

func (builder *CaptureBuilder) AppendBlackSwanCrash() {
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99, 5, 101, 5)
	builder.AppendTicker(testSymbolPrimary, 55, 50, 60, -45)
	builder.AppendBookSnapshot(testSymbolPrimary, 50, 1, 60, 1)
	builder.AppendSellTrade(testSymbolPrimary, 52, 25)
	builder.AppendTicker(testSymbolPrimary, 58, 54, 62, -40)
	builder.AppendBookSnapshot(testSymbolPrimary, 54, 8, 62, 8)
}
