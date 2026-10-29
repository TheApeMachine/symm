package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
)

func TestReadHistoryRestoresVirtualLot(t *testing.T) {
	t.Parallel()

	stored := data.NewValue("virtual", "open", "BTC/USD").Append(&Position{
		ID:        "lot-1",
		Symbol:    "BTC/USD",
		Virtual:   true,
		EntryPath: "R08.R01/R07.R00",
	}, "position")

	hit, ok := readHistory(stored)
	if !ok {
		t.Fatal("virtual history should decode")
	}

	if hit.role != "open" || !hit.position.Virtual || hit.position.EntryPath != "R08.R01/R07.R00" {
		t.Fatalf("hit = %+v", hit.position)
	}
}

func TestKeepHistoryPrefersNewerSnapshot(t *testing.T) {
	t.Parallel()

	older := data.NewValue("position", "open", "ETH/USD")
	older.Epoch = 1
	newer := data.NewValue("position", "open", "ETH/USD")
	newer.Epoch = 2

	open := map[string]historyHit{}
	keepHistory(open, "ETH/USD", historyHit{
		position: &Position{Symbol: "ETH/USD", EntryPath: "old"},
		value:    older,
	})
	keepHistory(open, "ETH/USD", historyHit{
		position: &Position{Symbol: "ETH/USD", EntryPath: "new"},
		value:    newer,
	})

	if open["ETH/USD"].position.EntryPath != "new" {
		t.Fatal("newer open snapshot should replace the older one")
	}

	keepHistory(open, "ETH/USD", historyHit{
		position: &Position{Symbol: "ETH/USD", EntryPath: "stale"},
		value:    older,
	})

	if open["ETH/USD"].position.EntryPath != "new" {
		t.Fatal("an older snapshot must not cover the restored lot")
	}
}

func TestRestoreSkipsLotAlreadyClosed(t *testing.T) {
	t.Parallel()

	open := historyHit{position: &Position{ID: "lot-zec", Symbol: "ZEC/USD"}}
	settled := map[string]historyHit{
		"lot-zec": {position: &Position{ID: "lot-zec"}},
	}

	if restoreOpen(open, settled) {
		t.Fatal("a lot with a closed record must not be restored as open")
	}

	fresh := historyHit{position: &Position{ID: "lot-new", Symbol: "ZEC/USD"}}
	if !restoreOpen(fresh, settled) {
		t.Fatal("a different lot on the same symbol is still open")
	}
}

func TestBindRestoresStopLoss(t *testing.T) {
	t.Parallel()

	position := &Position{Symbol: "BTC/USD"}
	desk := &Desk{}
	desk.bind(position)
	if position.stopLoss == nil {
		t.Fatal("expected stoploss after bind")
	}
}

func TestPrepareRestoredGivesExitPath(t *testing.T) {
	t.Parallel()

	position := &Position{EntryPath: "R08.R01/R07.R00"}
	prepareRestored(position)

	if position.ExitPath != position.EntryPath {
		t.Fatalf("exit path = %q", position.ExitPath)
	}
}

func TestOrdersToResumeSkipsFilledEntry(t *testing.T) {
	t.Parallel()

	position := &Position{
		Symbol: "BTC/USD",
		EntryOrder: &spot.OrderRequest{
			ClOrdId: "buy-1",
		},
		ExitOrder: &spot.OrderRequest{
			ClOrdId: "sell-1",
		},
		Fills: []*kraken.ExecutionData{{
			Side:    "buy",
			LastQty: decimal.NewFromFloat64(1),
		}},
	}

	open := spot.OpenOrdersResult{Open: map[string]spot.Order{
		"ord-buy":  {ClOrdID: "buy-1"},
		"ord-sell": {ClOrdID: "sell-1"},
	}}

	got := ordersToResume(position, open)
	if len(got) != 1 || got[0].clientOrderID != "sell-1" || got[0].orderID != "ord-sell" {
		t.Fatalf("resume = %+v", got)
	}

	if got[0].side != "sell" || got[0].pair != "BTC/USD" {
		t.Fatalf("resume = %+v", got)
	}
}

func TestLaterHistoryUsesTimestamp(t *testing.T) {
	t.Parallel()

	left := data.NewValue("position", "open", "BTC/USD")
	right := data.NewValue("position", "open", "BTC/USD")
	left.Timestamp = uint64(time.Now().Add(-time.Second).UnixNano())
	right.Timestamp = left.Timestamp + 1

	if !laterHistory(right, left) {
		t.Fatal("later timestamp should win")
	}

	if laterHistory(left, right) {
		t.Fatal("earlier timestamp should not win")
	}
}

type fillDuringWrite struct {
	desk *Desk
}

func (fillDuringWrite) Read(...string) ([]byte, error) { return nil, nil }

func (transport fillDuringWrite) Write([]byte) error {
	transport.desk.Apply(&kraken.Execution{
		Data: []kraken.ExecutionData{{
			Symbol:    "ZEC/USD",
			Side:      "buy",
			LastQty:   decimal.NewFromFloat64(0.03),
			CumQty:    decimal.NewFromFloat64(0.03),
			LastPrice: decimal.NewFromFloat64(1290.78),
			AvgPrice:  decimal.NewFromFloat64(1290.78),
		}},
	})
	return nil
}

func TestPaperFillDuringSendSetsEntry(t *testing.T) {
	t.Parallel()

	desk := &Desk{positions: NewCache(t.Context())}
	transport := fillDuringWrite{desk: desk}
	desk.transport = transport

	position := NewPosition(t.Context(), "ZEC/USD", "R01", true, false, nil, transport, nil)
	desk.positions.Add("ZEC/USD", position)

	if err := position.Enter("R01", decimal.NewFromFloat64(0.03)); err != nil {
		t.Fatal(err)
	}

	if position.EntryPrice == nil || position.EntryPrice.Float64() != 1290.78 {
		t.Fatalf("entry = %v", position.EntryPrice)
	}
}

type historyLedger struct {
	trades spot.TradesHistoryResult
}

func (historyLedger) Read(...string) ([]byte, error) { return nil, nil }
func (historyLedger) Write([]byte) error             { return nil }
func (ledger historyLedger) TradesHistory() (spot.TradesHistoryResult, error) {
	return ledger.trades, nil
}

func TestReconcilePaperFillSetsEntry(t *testing.T) {
	t.Parallel()

	opened := time.Date(2026, 10, 27, 15, 24, 44, 0, time.UTC)
	desk := &Desk{
		positions: NewCache(t.Context()),
		transport: historyLedger{trades: spot.TradesHistoryResult{
			Trades: map[string]spot.Trade{
				"PAPER-00002": {
					OrderID: "PAPER-00001",
					Pair:    "ZECUSD",
					Type:    "buy",
					Price:   decimal.NewFromFloat64(1290.78),
					Volume:  decimal.NewFromFloat64(0.03),
					Cost:    decimal.NewFromFloat64(38.72),
					Fee:     decimal.NewFromFloat64(0.31),
					Time:    decimal.NewFromFloat64(float64(opened.Unix()) + 0.7),
				},
				"PAPER-OLD": {
					OrderID: "PAPER-OLD",
					Pair:    "ZECUSD",
					Type:    "buy",
					Price:   decimal.NewFromFloat64(100),
					Volume:  decimal.NewFromFloat64(1),
					Time:    decimal.NewFromFloat64(float64(opened.Add(-time.Hour).Unix())),
				},
			},
		}},
	}

	position := &Position{
		Symbol:   "ZEC/USD",
		OpenedAt: opened,
		Qty:      decimal.NewFromFloat64(0.03),
	}
	desk.positions.Restore(position)
	desk.reconcilePaperFills()

	if position.EntryPrice == nil || position.EntryPrice.Float64() != 1290.78 {
		t.Fatalf("entry = %v", position.EntryPrice)
	}

	if len(position.Fills) != 1 || position.Fills[0].ExecID != "PAPER-00002" {
		t.Fatalf("fills = %+v", position.Fills)
	}
}

type walletLedger struct {
	trades spot.TradesHistoryResult
	wallet *kraken.Balance
	open   spot.OpenOrdersResult
	writes int
}

func (walletLedger) Read(...string) ([]byte, error) { return nil, nil }

func (ledger *walletLedger) Write([]byte) error {
	ledger.writes++
	return nil
}

func (ledger *walletLedger) TradesHistory() (spot.TradesHistoryResult, error) {
	return ledger.trades, nil
}

func (ledger *walletLedger) Balances() (*kraken.Balance, error) {
	return ledger.wallet, nil
}

func (ledger *walletLedger) OpenOrders() (spot.OpenOrdersResult, error) {
	return ledger.open, nil
}

func TestRetireFlatPaperDropsSoldLots(t *testing.T) {
	t.Parallel()

	aprOpen := time.Date(2026, 10, 27, 15, 53, 26, 0, time.UTC)
	xmnOpen := time.Date(2026, 10, 27, 16, 36, 20, 0, time.UTC)
	ledger := &walletLedger{
		wallet: kraken.NewBalanceFromMap(map[string]*decimal.Decimal{
			"APR":  decimal.NewFromInt64(0),
			"XMN":  decimal.NewFromInt64(0),
			"FET":  decimal.NewFromFloat64(150.08518924),
			"OSMO": decimal.NewFromInt64(0),
			"USD":  decimal.NewFromFloat64(102.59),
		}),
		open: spot.OpenOrdersResult{Open: map[string]spot.Order{
			"working": {Description: &spot.OrderDescription{Pair: "OSMOUSD"}},
		}},
		trades: spot.TradesHistoryResult{Trades: map[string]spot.Trade{
			"PAPER-00014": {
				OrderID: "PAPER-00013",
				Pair:    "APRUSD",
				Type:    "sell",
				Price:   decimal.NewFromFloat64(0.0395),
				Volume:  decimal.NewFromFloat64(79.92795),
				Time:    decimal.NewFromFloat64(float64(time.Date(2026, 10, 27, 16, 31, 42, 0, time.UTC).Unix())),
			},
			"PAPER-00020": {
				OrderID: "PAPER-00019",
				Pair:    "XMNUSD",
				Type:    "sell",
				Price:   decimal.NewFromFloat64(0.000321),
				Volume:  decimal.NewFromFloat64(62848.81846),
				Time:    decimal.NewFromFloat64(float64(time.Date(2026, 10, 27, 16, 45, 12, 0, time.UTC).Unix())),
			},
		}},
	}

	desk := &Desk{
		positions: NewCache(t.Context()),
		transport: ledger,
	}

	apr := &Position{Symbol: "APR/USD", OpenedAt: aprOpen, Qty: decimal.NewFromFloat64(79.92795)}
	xmn := &Position{Symbol: "XMN/USD", OpenedAt: xmnOpen, Qty: decimal.NewFromFloat64(62848.81846)}
	fet := &Position{Symbol: "FET/USD", Qty: decimal.NewFromFloat64(150.08518924)}
	osmo := &Position{Symbol: "OSMO/USD", Qty: decimal.NewFromFloat64(192)}
	desk.positions.Restore(apr)
	desk.positions.Restore(xmn)
	desk.positions.Restore(fet)
	desk.positions.Restore(osmo)

	desk.retireFlatPaper()

	if desk.positions.Get("APR/USD", true) != nil || desk.positions.Get("XMN/USD", true) != nil {
		t.Fatal("sold assets must not stay open on the desk")
	}

	if desk.positions.Get("FET/USD", true) == nil {
		t.Fatal("a held asset must stay open")
	}

	if desk.positions.Get("OSMO/USD", true) == nil {
		t.Fatal("a flat asset with a working order must stay open")
	}

	if ledger.writes != 0 {
		t.Fatalf("closing a flat lot sent %d orders", ledger.writes)
	}

	if apr.ExitPrice == nil || apr.ExitPrice.Float64() != 0.0395 {
		t.Fatalf("APR exit = %v", apr.ExitPrice)
	}

	if xmn.ClosedAt == nil || xmn.Qty == nil || xmn.Qty.Sign() != 0 {
		t.Fatalf("XMN closed=%v qty=%v", xmn.ClosedAt, xmn.Qty)
	}
}

func TestOpenPositionsStayInSymbolOrder(t *testing.T) {
	t.Parallel()

	cache := NewCache(t.Context())
	cache.Add("EDGE/USD", &Position{ID: "edge", Symbol: "EDGE/USD"})
	cache.Add("ASTER/USD", &Position{ID: "aster", Symbol: "ASTER/USD"})
	cache.Add("PTB/USD", &Position{ID: "ptb", Symbol: "PTB/USD"})

	got := cache.OpenPositions()
	if len(got) != 3 || got[0].Symbol != "ASTER/USD" || got[1].Symbol != "EDGE/USD" || got[2].Symbol != "PTB/USD" {
		symbols := make([]string, len(got))
		for i, position := range got {
			symbols[i] = position.Symbol
		}
		t.Fatalf("order = %v", symbols)
	}
}

func TestLotUpdatesAddressEachLot(t *testing.T) {
	t.Parallel()

	frames := lotUpdates([]*Position{
		{ID: "edge", Symbol: "EDGE/USD"},
		{ID: "aster", Symbol: "ASTER/USD"},
		nil,
		{Symbol: ""},
		{Symbol: "BTC/USD", Virtual: true},
	})

	if len(frames) != 3 {
		t.Fatalf("frames = %d", len(frames))
	}

	if frames[0].Scope != "aster" || frames[1].Scope != "virtual:BTC/USD" || frames[2].Scope != "edge" {
		t.Fatalf("scopes = %s %s %s", frames[0].Scope, frames[1].Scope, frames[2].Scope)
	}

	for _, frame := range frames {
		if frame.Scope == "all" {
			t.Fatal("a mark tick must not replace the book snapshot")
		}

		var got Position
		frame.Lookup(&got, "position")
		if frame.Err != nil || got.Symbol == "" {
			t.Fatalf("lot %s did not decode: %v", frame.Scope, frame.Err)
		}
	}
}
