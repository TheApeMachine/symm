package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

type stubQuote map[string]struct {
	last float64
	bid  float64
	ask  float64
}

func (stub stubQuote) Last(symbol string) (float64, bool) {
	row, ok := stub[symbol]

	if !ok {
		return 0, false
	}

	return row.last, true
}

func (stub stubQuote) Quote(symbol string) (float64, float64, float64, float64, bool) {
	row, ok := stub[symbol]

	if !ok {
		return 0, 0, 0, 0, false
	}

	return row.last, row.bid, row.ask, 0, true
}

func (stub stubQuote) BookDepth(symbol string) ([]market.BookLevel, []market.BookLevel, bool) {
	row, ok := stub[symbol]

	if !ok || row.bid <= 0 || row.ask <= 0 {
		return nil, nil, false
	}

	return []market.BookLevel{{Price: row.bid, Volume: 1}},
		[]market.BookLevel{{Price: row.ask, Volume: 1}},
		true
}

type captureStream struct {
	events []map[string]any
}

func (stream *captureStream) TradeEnter(payload map[string]any) {
	stream.events = append(stream.events, payload)
}

func (stream *captureStream) TradeExit(payload map[string]any) {
	stream.events = append(stream.events, payload)
}

func (stream *captureStream) StopRatchet(payload map[string]any) {
	stream.events = append(stream.events, payload)
}

func TestPortfolioTryEnter(t *testing.T) {
	config.System.MaxSlots = 2
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	stream := &captureStream{}
	portfolio.BindStream(stream)

	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	event, ok := portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Source: "hawkes",
		Regime: "momentum",
		Reason: "cluster_buy",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	portfolio.Emit(event)

	status := portfolio.Status(quotes)

	if status.OpenCount != 1 {
		t.Fatalf("expected one open position, got %d", status.OpenCount)
	}

	openPnL, ok := status.Positions[0]["open_pnl_eur"].(float64)

	if !ok {
		t.Fatalf(
			"expected open_pnl_eur on status position, got %T",
			status.Positions[0]["open_pnl_eur"],
		)
	}

	_ = openPnL

	if wallet.Balance >= 200 {
		t.Fatalf("expected wallet debit after entry, balance=%f", wallet.Balance)
	}

	if len(stream.events) != 1 || stream.events[0]["event"] != "trade_enter" {
		t.Fatalf("expected trade_enter event, got %+v", stream.events)
	}
}

func TestPortfolioMarkStopExit(t *testing.T) {
	config.System.MaxSlots = 1
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	stream := &captureStream{}
	portfolio.BindStream(stream)

	entryQuotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	event, ok := portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, entryQuotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	portfolio.Emit(event)

	stopPrice, _ := stream.events[0]["stop"].(float64)

	if stopPrice <= 0 {
		t.Fatalf("expected stop on trade_enter, got %+v", stream.events[0])
	}

	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(now.Add(2*time.Millisecond), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected exit event")
	}

	for _, markEvent := range events {
		portfolio.Emit(&markEvent)
	}

	status := portfolio.Status(exitQuotes)

	convey.Convey("Given a position stopped out after min hold", t, func() {
		convey.Convey("It should close the position and publish trade_exit", func() {
			convey.So(status.OpenCount, convey.ShouldEqual, 0)
			convey.So(status.TradeCount, convey.ShouldEqual, 1)
			convey.So(events[len(events)-1].Name, convey.ShouldEqual, "trade_exit")
		})
	})
}

func TestPortfolioClosedPnLIncludesEntryFee(t *testing.T) {
	config.System.MaxSlots = 1
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	entryQuotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, entryQuotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	position := portfolio.positions["PUMP/EUR"]

	if position.EntryFeeEUR <= 0 {
		t.Fatalf("expected entry fee stored on position, got %v", position.EntryFeeEUR)
	}

	stopPrice := position.StopPrice
	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(now.Add(2*time.Millisecond), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected exit event")
	}

	exitPnL, _ := events[len(events)-1].Payload["pnl_eur"].(float64)
	status := portfolio.Status(exitQuotes)

	if exitPnL >= 0 {
		t.Fatalf("expected losing exit pnl, got %v", exitPnL)
	}

	if status.ClosedPnLEUR != exitPnL {
		t.Fatalf("expected closed pnl %v, got %v", exitPnL, status.ClosedPnLEUR)
	}

	if wallet.Balance != 200+status.ClosedPnLEUR {
		t.Fatalf(
			"expected wallet to reflect entry and exit fees, balance=%v closed=%v",
			wallet.Balance,
			status.ClosedPnLEUR,
		)
	}
}

func TestPortfolioScalpHoldUsesRegimeHold(t *testing.T) {
	config.System.ScalpHoldBeforeExit = 15 * time.Second
	config.System.MinHoldBeforeRotate = time.Minute

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Regime: "momentum",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	stopPrice, _ := portfolio.positions["PUMP/EUR"].StopPrice, true
	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(now.Add(5*time.Second), exitQuotes)

	if len(events) != 0 {
		t.Fatal("expected hold to block exit before scalp hold elapsed")
	}

	events = portfolio.Mark(now.Add(16*time.Second), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected exit after scalp hold elapsed")
	}
}

func TestPortfolioShortEntry(t *testing.T) {
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Regime: "dump",
		Score:  0.8,
		Price:  100,
		Side:   positionShort,
	}, quotes)

	if !ok {
		t.Fatal("expected short entry")
	}

	position := portfolio.positions["PUMP/EUR"]

	if position.Side != positionShort {
		t.Fatalf("expected short side, got %v", position.Side)
	}
}

func BenchmarkPortfolioMark(b *testing.B) {
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)

	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, _ = portfolio.TryEnter(now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, quotes)

	b.ReportAllocs()

	for b.Loop() {
		portfolio.Mark(now.Add(time.Second), quotes)
	}
}
