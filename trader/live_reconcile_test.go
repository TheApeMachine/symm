package trader

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/rest"
)

func TestPaperSimulatedFillRejectsInsufficientDepth(t *testing.T) {
	convey.Convey("Given shallow ask depth below coverage", t, func() {
		_, _, _, err := paperSimulatedFill(
			"buy",
			100,
			0,
			100,
			99.9,
			100.1,
			nil,
			[]market.BookLevel{{Price: 100, Volume: 0.1}},
		)

		convey.Convey("It should reject the entry", func() {
			convey.So(err, convey.ShouldEqual, errInsufficientDepth)
		})
	})
}

func TestJournalOpenEntry(t *testing.T) {
	tempDir := t.TempDir()
	journal := NewOrderJournal(tempDir)
	journal.RecordEntry(OrderJournalEntry{
		Event:       "trade_enter",
		Symbol:      "BTC/EUR",
		OrderID:     "entry-1",
		StopOrderID: "stop-1",
		FillPrice:   100,
		NotionalEUR: 50,
		TS:          time.Unix(1_700_000_000, 0).UTC(),
	})
	journal.RecordEntry(OrderJournalEntry{
		Event:  "trade_exit",
		Symbol: "BTC/EUR",
		TS:     time.Unix(1_700_000_100, 0).UTC(),
	})
	journal.RecordEntry(OrderJournalEntry{
		Event:       "trade_enter",
		Symbol:      "BTC/EUR",
		OrderID:     "entry-2",
		StopOrderID: "stop-2",
		FillPrice:   110,
		NotionalEUR: 55,
		TS:          time.Unix(1_700_000_200, 0).UTC(),
	})

	journal.LoadFromDisk()

	entry, ok := journalOpenEntry(journal, "BTC/EUR")

	if !ok {
		t.Fatal("expected open journal entry")
	}

	if entry.OrderID != "entry-2" || entry.StopOrderID != "stop-2" {
		t.Fatalf("unexpected open entry: %+v", entry)
	}
}

func TestReconcileLiveRecoversPosition(t *testing.T) {
	wallet := NewWallet(CryptoWallet, "EUR", 0, 0.26)
	portfolio := NewPortfolio(wallet)
	portfolio.BindBroker(NewKrakenBroker(nil, nil, 0.26))

	journal := NewOrderJournal("")
	journal.RecordEntry(OrderJournalEntry{
		Event:       "trade_enter",
		Symbol:      "BTC/EUR",
		OrderID:     "entry-1",
		StopOrderID: "stop-1",
		FillPrice:   100,
		NotionalEUR: 50,
		TS:          time.Unix(1_700_000_000, 0).UTC(),
	})

	balance := &rest.Balance{
		Result: map[string]string{
			"ZEUR": "150",
			"XXBT": "0.5",
		},
	}

	pairIndex := map[string]asset.Pair{
		"BTC/EUR": {Base: "XXBT", Quote: "ZEUR", Wsname: "BTC/EUR"},
	}

	if err := portfolio.ReconcileLive(wallet, balance, pairIndex, journal); err != nil {
		t.Fatal(err)
	}

	position := portfolio.positions["BTC/EUR"]

	if position == nil {
		t.Fatal("expected recovered position")
	}

	if position.BaseQty != 0.5 {
		t.Fatalf("expected base qty 0.5, got %v", position.BaseQty)
	}

	if wallet.AvailableBase("BTC/EUR") != 0.5 {
		t.Fatalf("expected inventory synced, got %v", wallet.AvailableBase("BTC/EUR"))
	}

	if wallet.Balance != 150 {
		t.Fatalf("expected EUR balance 150, got %v", wallet.Balance)
	}
}

func TestOrderJournalLoadFromDisk(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "orders.jsonl")
	line := `{"ts":"2024-01-01T00:00:00Z","event":"trade_enter","symbol":"BTC/EUR","order_id":"A","stop_order_id":"S"}` + "\n"

	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	journal := NewOrderJournal(tempDir)
	journal.LoadFromDisk()

	if len(journal.Entries()) != 1 {
		t.Fatalf("expected one journal row, got %d", len(journal.Entries()))
	}
}
