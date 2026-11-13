package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChannelTrades(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelTrades is read", func() {
			Convey("it is trades", func() {
				So(ChannelTrades, ShouldEqual, "trades")
			})
		})
	})
}

func TestChannelLevel3(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelLevel3 is read", func() {
			Convey("it is level3", func() {
				So(ChannelLevel3, ShouldEqual, "level3")
			})
		})
	})
}

func TestChannelFuturesTickers(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelFuturesTickers is read", func() {
			Convey("it is futures_tickers", func() {
				So(ChannelFuturesTickers, ShouldEqual, "futures_tickers")
			})
		})
	})
}

func TestChannelFuturesTrades(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelFuturesTrades is read", func() {
			Convey("it is futures_trades", func() {
				So(ChannelFuturesTrades, ShouldEqual, "futures_trades")
			})
		})
	})
}

func TestChannelSignals(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelSignals is read", func() {
			Convey("it is signals", func() {
				So(ChannelSignals, ShouldEqual, "signals")
			})
		})
	})
}

func TestChannelPerspectives(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelPerspectives is read", func() {
			Convey("it is perspectives", func() {
				So(ChannelPerspectives, ShouldEqual, "perspectives")
			})
		})
	})
}

func TestChannelDecisions(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelDecisions is read", func() {
			Convey("it is decisions", func() {
				So(ChannelDecisions, ShouldEqual, "decisions")
			})
		})
	})
}

func TestChannelExecutions(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelExecutions is read", func() {
			Convey("it is executions", func() {
				So(ChannelExecutions, ShouldEqual, "executions")
			})
		})
	})
}

func TestChannelOrders(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelOrders is read", func() {
			Convey("it is orders", func() {
				So(ChannelOrders, ShouldEqual, "orders")
			})
		})
	})
}

func TestChannelPositions(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelPositions is read", func() {
			Convey("it is positions", func() {
				So(ChannelPositions, ShouldEqual, "positions")
			})
		})
	})
}

func TestChannelPnl(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelPnl is read", func() {
			Convey("it is pnl", func() {
				So(ChannelPnl, ShouldEqual, "pnl")
			})
		})
	})
}

func TestChannelTelemetry(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ChannelTelemetry is read", func() {
			Convey("it is telemetry", func() {
				So(ChannelTelemetry, ShouldEqual, "telemetry")
			})
		})
	})
}

func BenchmarkChannelTrades(b *testing.B) {
	for b.Loop() {
		_ = ChannelTrades
	}
}

func BenchmarkChannelLevel3(b *testing.B) {
	for b.Loop() {
		_ = ChannelLevel3
	}
}

func BenchmarkChannelFuturesTickers(b *testing.B) {
	for b.Loop() {
		_ = ChannelFuturesTickers
	}
}

func BenchmarkChannelFuturesTrades(b *testing.B) {
	for b.Loop() {
		_ = ChannelFuturesTrades
	}
}

func BenchmarkChannelSignals(b *testing.B) {
	for b.Loop() {
		_ = ChannelSignals
	}
}

func BenchmarkChannelPerspectives(b *testing.B) {
	for b.Loop() {
		_ = ChannelPerspectives
	}
}

func BenchmarkChannelDecisions(b *testing.B) {
	for b.Loop() {
		_ = ChannelDecisions
	}
}

func BenchmarkChannelExecutions(b *testing.B) {
	for b.Loop() {
		_ = ChannelExecutions
	}
}

func BenchmarkChannelOrders(b *testing.B) {
	for b.Loop() {
		_ = ChannelOrders
	}
}

func BenchmarkChannelPositions(b *testing.B) {
	for b.Loop() {
		_ = ChannelPositions
	}
}

func BenchmarkChannelPnl(b *testing.B) {
	for b.Loop() {
		_ = ChannelPnl
	}
}

func BenchmarkChannelTelemetry(b *testing.B) {
	for b.Loop() {
		_ = ChannelTelemetry
	}
}
