package trader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestPortfolioStoreRoundTrip(t *testing.T) {
	convey.Convey("Given an open paper portfolio", t, func() {
		tempDir := t.TempDir()
		wallet := NewWallet(PaperWallet, "EUR", 180, 0.26)
		wallet.ReservedEUR = 5
		_ = wallet.CreditBase("PUMP/EUR", 0.5)

		portfolio := NewPortfolio(wallet)
		openedAt := time.Unix(1_700_000_000, 0).UTC()
		portfolio.positions["PUMP/EUR"] = &Position{
			Symbol:      "PUMP/EUR",
			Side:        positionLong,
			FillPrice:   100,
			StopPrice:   95,
			PeakPrice:   101,
			NotionalEUR: 50,
			BaseQty:     0.5,
			StopOrderID: "paper-stop-1",
			OpenedAt:    openedAt,
		}
		portfolio.closedPnL = 1.5
		portfolio.tradeCount = 2
		portfolio.wins = 1

		store := NewPortfolioStore(tempDir)

		convey.Convey("It should persist and restore positions with inventory", func() {
			err := store.Save(portfolio)
			convey.So(err, convey.ShouldBeNil)

			restoredWallet := NewWallet(PaperWallet, "EUR", 999, 0.26)
			restored := NewPortfolio(restoredWallet)

			err = store.Restore(restored)
			convey.So(err, convey.ShouldBeNil)
			convey.So(restoredWallet.Balance, convey.ShouldEqual, 180)
			convey.So(restoredWallet.ReservedEUR, convey.ShouldEqual, 5)
			convey.So(restoredWallet.AvailableBase("PUMP/EUR"), convey.ShouldEqual, 0.5)
			convey.So(len(restored.positions), convey.ShouldEqual, 1)
			convey.So(restored.positions["PUMP/EUR"].StopOrderID, convey.ShouldEqual, "paper-stop-1")
			convey.So(restored.closedPnL, convey.ShouldEqual, 1.5)
		})
	})
}

func TestPortfolioExitDebitsInventory(t *testing.T) {
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

	if wallet.AvailableBase("PUMP/EUR") != position.BaseQty {
		t.Fatalf(
			"expected inventory %v, got %v",
			position.BaseQty,
			wallet.AvailableBase("PUMP/EUR"),
		)
	}

	stopPrice := position.StopPrice
	exitQuotes := stubQuote{
		"PUMP/EUR": {last: stopPrice * 0.99, bid: 90, ask: 90.2},
	}

	events := portfolio.Mark(context.Background(), now.Add(2*time.Millisecond), exitQuotes)

	if len(events) == 0 {
		t.Fatal("expected exit event")
	}

	if wallet.AvailableBase("PUMP/EUR") != 0 {
		t.Fatalf("expected inventory cleared after exit, got %v", wallet.AvailableBase("PUMP/EUR"))
	}
}

func TestPortfolioStoreRestoreSkipsLiveWallet(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "portfolio.json")
	payload := []byte(`{"cash_eur":10,"positions":[{"symbol":"BTC/EUR","side":1,"base_qty":1,"opened_at":"2024-01-01T00:00:00Z"}]}`)

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	wallet := NewWallet(CryptoWallet, "EUR", 500, 0.26)
	portfolio := NewPortfolio(wallet)
	store := NewPortfolioStore(tempDir)

	if err := store.Restore(portfolio); err != nil {
		t.Fatal(err)
	}

	if len(portfolio.positions) != 0 {
		t.Fatal("expected live wallet restore to skip persisted positions")
	}

	if wallet.Balance != 500 {
		t.Fatalf("expected live wallet balance unchanged, got %v", wallet.Balance)
	}
}
