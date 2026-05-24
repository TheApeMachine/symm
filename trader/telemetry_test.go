package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

type recordingUIStream struct {
	status        map[string]any
	scoreboard    []map[string]any
	decisionTrace map[string]any
}

func (stream *recordingUIStream) SignalScore(string, float64) {}

func (stream *recordingUIStream) EnginePulse(map[string]any) {}

func (stream *recordingUIStream) Status(payload map[string]any) {
	stream.status = payload
}

func (stream *recordingUIStream) Scoreboard(
	line, median, mad float64,
	targets []map[string]any,
) {
	stream.scoreboard = targets
}

func (stream *recordingUIStream) DecisionTrace(payload map[string]any) {
	stream.decisionTrace = payload
}

func TestPublishDashboard(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	stream := &recordingUIStream{}

	crypto := testCrypto(t, stubPrices{"PUMP/EUR": 100}, &stubSignal{})
	crypto.wallet = wallet
	crypto.portfolio = NewPortfolio(wallet)
	crypto.BindUIStream(stream)

	crypto.PrimeDashboard()

	convey.Convey("Given a primed crypto trader", t, func() {
		convey.Convey("It should publish wallet status for the dashboard header", func() {
			convey.So(stream.status["equity_eur"], convey.ShouldEqual, 200)
			convey.So(stream.status["cash_eur"], convey.ShouldEqual, 200)
			convey.So(stream.status["event"], convey.ShouldEqual, "status")
		})

		convey.Convey("It should cache bootstrap frames for new websocket clients", func() {
			bootstrap := crypto.Bootstrap()

			convey.So(len(bootstrap), convey.ShouldEqual, 3)
			convey.So(bootstrap[0]["event"], convey.ShouldEqual, "status")
			convey.So(bootstrap[1]["event"], convey.ShouldEqual, "scoreboard")
			convey.So(bootstrap[2]["event"], convey.ShouldEqual, "decision_trace")
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
