package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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

type captureUI struct {
	group      *qpool.BroadcastGroup
	subscriber *qpool.Subscriber
}

func newCaptureUI(t *testing.T) *captureUI {
	t.Helper()

	ctx := context.Background()
	group, err := qpool.NewBroadcastGroup(ctx, "portfolio-test", time.Minute)

	if err != nil {
		t.Fatalf("broadcast group: %v", err)
	}

	return &captureUI{
		group:      group,
		subscriber: group.Subscribe("capture", 8),
	}
}

func (capture *captureUI) waitEvent(t *testing.T) map[string]any {
	t.Helper()

	select {
	case value := <-capture.subscriber.Incoming:
		payload, _ := value.Value.(map[string]any)
		return payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for portfolio event")
	}

	return nil
}

func TestPortfolioTryEnter(t *testing.T) {
	config.System.MaxSlots = 2
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	capture := newCaptureUI(t)
	portfolio.BindUI(capture.group)

	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	event, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
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
	tradeEnter := capture.waitEvent(t)

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

	if tradeEnter["event"] != "trade_enter" {
		t.Fatalf("expected trade_enter event, got %+v", tradeEnter)
	}
}

func TestPortfolioMarkStopExit(t *testing.T) {
	config.System.MaxSlots = 1
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	capture := newCaptureUI(t)
	portfolio.BindUI(capture.group)

	entryQuotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	event, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, entryQuotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	portfolio.Emit(event)
	tradeEnter := capture.waitEvent(t)

	stopPrice, _ := tradeEnter["stop"].(float64)

	if stopPrice <= 0 {
		t.Fatalf("expected stop on trade_enter, got %+v", tradeEnter)
	}

	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(context.Background(), now.Add(2*time.Millisecond), exitQuotes)

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
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
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

	events := portfolio.Mark(context.Background(), now.Add(2*time.Millisecond), exitQuotes)

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

func TestPortfolioEntryDebitsActualFillCost(t *testing.T) {
	config.System.MaxSlots = 1
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1
	config.System.MinHoldBeforeRotate = time.Millisecond

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	position := portfolio.positions["PUMP/EUR"]
	expectedProceeds := spotProceedsEUR(position.BaseQty, position.FillPrice)
	expectedDebit := expectedProceeds + position.EntryFeeEUR

	if position.NotionalEUR != expectedProceeds {
		t.Fatalf("expected notional=%v, got %v", expectedProceeds, position.NotionalEUR)
	}

	if position.StopOrderID == "" {
		t.Fatal("expected protective stop order id on paper entry")
	}

	if wallet.Balance != 200-expectedDebit {
		t.Fatalf("expected wallet balance %v, got %v", 200-expectedDebit, wallet.Balance)
	}
}

func TestHardStopIgnoresMinHold(t *testing.T) {
	config.System.ScalpHoldBeforeExit = 15 * time.Second
	config.System.MinHoldBeforeRotate = time.Minute

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Regime: "momentum",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	stopPrice := portfolio.positions["PUMP/EUR"].StopPrice
	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(context.Background(), now.Add(5*time.Second), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected hard stop to exit before scalp hold elapsed")
	}

	if events[len(events)-1].Name != "trade_exit" {
		t.Fatalf("expected trade_exit, got %+v", events[len(events)-1])
	}
}

func TestPortfolioScalpHoldBlocksEarlyExit(t *testing.T) {
	config.System.ScalpHoldBeforeExit = 15 * time.Second
	config.System.MinHoldBeforeRotate = time.Minute
	config.System.ExitUrgencyThreshold = 0.65

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	portfolio.BindExitAdvisor(stubExitAdvisor{urgency: 0.9, reason: "pressure_fade"})
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Regime: "momentum",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected portfolio entry")
	}

	exitQuotes := stubQuote{
		"PUMP/EUR": {last: 101, bid: 100.9, ask: 101.1},
	}

	events := portfolio.Mark(context.Background(), now.Add(5*time.Second), exitQuotes)

	for _, event := range events {
		if event.Name == "trade_exit" {
			t.Fatal("expected hold to block early exit before scalp hold elapsed")
		}
	}

	events = portfolio.Mark(context.Background(), now.Add(16*time.Second), exitQuotes)

	hasExit := false

	for _, event := range events {
		if event.Name == "trade_exit" {
			hasExit = true
		}
	}

	if !hasExit {
		t.Fatal("expected early exit after scalp hold elapsed")
	}
}

type stubExitAdvisor struct {
	urgency float64
	reason  string
}

func (advisor stubExitAdvisor) ExitUrgency(_ string, _ int) (float64, string) {
	return advisor.urgency, advisor.reason
}

func TestPortfolioShortEntry(t *testing.T) {
	config.System.MinHoldBeforeRotate = time.Millisecond
	config.System.AllowPaperShorts = true

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
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

func TestSpotBrokerRejectsShort(t *testing.T) {
	config.System.AllowPaperShorts = false

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
		Side:   positionShort,
	}, quotes)

	if ok {
		t.Fatal("expected short entry to be rejected when paper shorts are disabled")
	}
}

type blockingBroker struct {
	entered chan struct{}
	release chan struct{}
}

func (broker *blockingBroker) Live() bool { return true }

func (broker *blockingBroker) SupportsShort() bool { return false }

func (broker *blockingBroker) Enter(
	ctx context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
	close(broker.entered)

	select {
	case <-broker.release:
	case <-ctx.Done():
		return BrokerFill{}, ctx.Err()
	}

	return NewPaperBroker().Enter(ctx, request)
}

func (broker *blockingBroker) Exit(
	ctx context.Context,
	request BrokerExitRequest,
) (BrokerFill, error) {
	return NewPaperBroker().Exit(ctx, request)
}

func (broker *blockingBroker) AmendStop(
	ctx context.Context,
	request BrokerAmendStopRequest,
) error {
	return NewPaperBroker().AmendStop(ctx, request)
}

func TestLiveBrokerTimeoutDoesNotHoldPortfolioLock(t *testing.T) {
	config.System.MaxSlots = 2
	config.System.MaxSlotPct = 10
	config.System.MinCostEUR = 1

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	broker := &blockingBroker{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	portfolio.BindBroker(broker)

	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
	}

	now := time.Unix(1_700_000_000, 0)
	done := make(chan struct{})

	go func() {
		_, _ = portfolio.TryEnter(context.Background(), now, ExecutionDecision{
			Symbol: "PUMP/EUR",
			Score:  0.8,
			Price:  100,
		}, quotes)
		close(done)
	}()

	<-broker.entered

	statusDone := make(chan struct{})

	go func() {
		_ = portfolio.Status(quotes)
		close(statusDone)
	}()

	select {
	case <-statusDone:
	case <-time.After(time.Second):
		t.Fatal("portfolio lock was held during broker entry")
	}

	close(broker.release)
	<-done
}

func TestPortfolioMaxSpreadGate(t *testing.T) {
	config.System.MaxSpreadBPS = 5
	t.Cleanup(func() { config.System.MaxSpreadBPS = 0 })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99, ask: 101},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if ok {
		t.Fatal("expected wide spread to block entry")
	}
}

func TestPortfolioMaxDailyLossGate(t *testing.T) {
	config.System.MaxDailyLossEUR = 5
	config.System.MaxLossPerTradeEUR = 0
	config.System.MaxSpreadBPS = 0
	config.System.MinHoldBeforeRotate = time.Millisecond
	t.Cleanup(func() {
		config.System.MaxDailyLossEUR = 20
		config.System.MaxLossPerTradeEUR = 2
	})

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	portfolio := NewPortfolio(wallet)
	quotes := stubQuote{
		"PUMP/EUR": {last: 100, bid: 99.9, ask: 100.1},
		"DUMP/EUR": {last: 50, bid: 49.9, ask: 50.1},
	}

	now := time.Unix(1_700_000_000, 0)
	_, ok := portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, quotes)

	if !ok {
		t.Fatal("expected first entry")
	}

	stopPrice := portfolio.positions["PUMP/EUR"].StopPrice
	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
		"DUMP/EUR": {last: 50, bid: 49.9, ask: 50.1},
	}

	events := portfolio.Mark(context.Background(), now.Add(time.Millisecond), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected stop exit")
	}

	portfolio.mu.Lock()
	portfolio.dailyClosedPnL = -6
	portfolio.mu.Unlock()

	_, ok = portfolio.TryEnter(context.Background(), now.Add(time.Second), ExecutionDecision{
		Symbol: "DUMP/EUR",
		Score:  0.8,
		Price:  50,
	}, exitQuotes)

	if ok {
		t.Fatal("expected daily loss gate to block new entries")
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
	_, _ = portfolio.TryEnter(context.Background(), now, ExecutionDecision{
		Symbol: "PUMP/EUR",
		Score:  0.8,
		Price:  100,
	}, quotes)

	b.ReportAllocs()

	for b.Loop() {
		portfolio.Mark(context.Background(), now.Add(time.Second), quotes)
	}
}
