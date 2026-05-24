package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestPublishStatus(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	uiGroup := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	subscriber := uiGroup.Subscribe("test", 8)

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

	crypto, err := NewCrypto(
		ctx,
		pool,
		uiGroup,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.PrimeDashboard()

	var payload map[string]any

	select {
	case value := <-subscriber.Incoming:
		payload, _ = value.Value.(map[string]any)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for status")
	}

	convey.Convey("Given a primed crypto trader", t, func() {
		convey.Convey("It should publish wallet status for the dashboard header", func() {
			convey.So(payload["equity_eur"], convey.ShouldEqual, 200)
			convey.So(payload["cash_eur"], convey.ShouldEqual, 200)
			convey.So(payload["event"], convey.ShouldEqual, "status")
		})
	})
}

func TestStatusPayload(t *testing.T) {
	convey.Convey("Given portfolio telemetry", t, func() {
		payload := statusPayload(StatusSnapshot{
			EquityEUR:    210,
			CashEUR:      180,
			ClosedPnLEUR: 5,
			TradeCount:   4,
			WinRate:      0.5,
			OpenCount:    1,
			Positions: []map[string]any{
				{"symbol": "PUMP/EUR"},
			},
		})

		convey.Convey("It should expose the dashboard header fields", func() {
			convey.So(payload["equity_eur"], convey.ShouldEqual, 210)
			convey.So(payload["closed_pnl_eur"], convey.ShouldEqual, 5)
			convey.So(payload["open_count"], convey.ShouldEqual, 1)
			convey.So(payload["positions"], convey.ShouldHaveLength, 1)
		})
	})
}
