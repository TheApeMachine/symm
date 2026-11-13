package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourceType(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When source type constants are read", func() {
			Convey("each constant is the exact wire name", func() {
				So(string(SourceCorrelation), ShouldEqual, "correlation")
				So(string(SourceCVD), ShouldEqual, "cvd")
				So(string(SourceDepthFlow), ShouldEqual, "depthflow")
				So(string(SourceHawkes), ShouldEqual, "hawkes")
				So(string(SourceLeadLag), ShouldEqual, "leadlag")
				So(string(SourceLiquidity), ShouldEqual, "liquidity")
				So(string(SourceMorphology), ShouldEqual, "morphology")
				So(string(SourcePumpDump), ShouldEqual, "pumpdump")
				So(string(SourceSentiment), ShouldEqual, "sentiment")
				So(string(SourceToxicity), ShouldEqual, "toxicity")
			})
		})
	})
}

func TestSignalSources(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When SignalSources is read", func() {
			Convey("it lists every source type in order", func() {
				So(len(SignalSources), ShouldEqual, 10)
				So(SignalSources[0], ShouldEqual, SourceCorrelation)
				So(SignalSources[1], ShouldEqual, SourceCVD)
				So(SignalSources[2], ShouldEqual, SourceDepthFlow)
				So(SignalSources[3], ShouldEqual, SourceHawkes)
				So(SignalSources[4], ShouldEqual, SourceLeadLag)
				So(SignalSources[5], ShouldEqual, SourceLiquidity)
				So(SignalSources[6], ShouldEqual, SourceMorphology)
				So(SignalSources[7], ShouldEqual, SourcePumpDump)
				So(SignalSources[8], ShouldEqual, SourceSentiment)
				So(SignalSources[9], ShouldEqual, SourceToxicity)
			})
		})
	})
}

func TestSignalSourceStrings(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When SignalSourceStrings is read", func() {
			Convey("it lists every wire name in order", func() {
				So(SignalSourceStrings, ShouldResemble, []string{
					"correlation",
					"cvd",
					"depthflow",
					"hawkes",
					"leadlag",
					"liquidity",
					"morphology",
					"pumpdump",
					"sentiment",
					"toxicity",
				})
			})
		})
	})
}

func TestLogicSourceStrings(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When LogicSourceStrings is read", func() {
			Convey("it lists the logic sources in order", func() {
				So(LogicSourceStrings, ShouldResemble, []string{
					"category",
					"manifold",
					"resonance",
					"graph",
				})
			})
		})
	})
}

func TestTradeReceivers(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When TradeReceivers is read", func() {
			Convey("it lists trade-consuming sources in order", func() {
				So(len(TradeReceivers), ShouldEqual, 6)
				So(TradeReceivers[0], ShouldEqual, SourceCorrelation)
				So(TradeReceivers[1], ShouldEqual, SourceCVD)
				So(TradeReceivers[2], ShouldEqual, SourceHawkes)
				So(TradeReceivers[3], ShouldEqual, SourceLeadLag)
				So(TradeReceivers[4], ShouldEqual, SourcePumpDump)
				So(TradeReceivers[5], ShouldEqual, SourceSentiment)
			})
		})
	})
}

func TestTradeReceiverStrings(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When TradeReceiverStrings is read", func() {
			Convey("it lists trade receiver wire names in order", func() {
				So(TradeReceiverStrings, ShouldResemble, []string{
					"correlation",
					"cvd",
					"hawkes",
					"leadlag",
					"pumpdump",
					"sentiment",
				})
			})
		})
	})
}

func TestBookReceivers(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When BookReceivers is read", func() {
			Convey("it lists book-consuming sources in order", func() {
				So(len(BookReceivers), ShouldEqual, 2)
				So(BookReceivers[0], ShouldEqual, SourceDepthFlow)
				So(BookReceivers[1], ShouldEqual, SourceLiquidity)
			})
		})
	})
}

func TestLevel3Receivers(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Level3Receivers is read", func() {
			Convey("it lists level3-consuming sources in order", func() {
				So(len(Level3Receivers), ShouldEqual, 4)
				So(Level3Receivers[0], ShouldEqual, SourceDepthFlow)
				So(Level3Receivers[1], ShouldEqual, SourceMorphology)
				So(Level3Receivers[2], ShouldEqual, SourceToxicity)
				So(Level3Receivers[3], ShouldEqual, SourcePumpDump)
			})
		})
	})
}

func TestLevel3ReceiverStrings(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Level3ReceiverStrings is read", func() {
			Convey("it lists level3 receiver wire names in order", func() {
				So(Level3ReceiverStrings, ShouldResemble, []string{
					"depthflow",
					"morphology",
					"toxicity",
					"pumpdump",
				})
			})
		})
	})
}

func TestAcceptedBookReceivers(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When AcceptedBookReceivers is read", func() {
			Convey("it is BookReceivers then Level3Receivers", func() {
				So(len(AcceptedBookReceivers), ShouldEqual, 6)
				So(AcceptedBookReceivers[0], ShouldEqual, SourceDepthFlow)
				So(AcceptedBookReceivers[1], ShouldEqual, SourceLiquidity)
				So(AcceptedBookReceivers[2], ShouldEqual, SourceDepthFlow)
				So(AcceptedBookReceivers[3], ShouldEqual, SourceMorphology)
				So(AcceptedBookReceivers[4], ShouldEqual, SourceToxicity)
				So(AcceptedBookReceivers[5], ShouldEqual, SourcePumpDump)
			})
		})
	})
}

func BenchmarkSourceType(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = string(SourcePumpDump)
	}
}

func BenchmarkSignalSources(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(SignalSources)
	}
}

func BenchmarkSignalSourceStrings(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(SignalSourceStrings)
	}
}

func BenchmarkLogicSourceStrings(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(LogicSourceStrings)
	}
}

func BenchmarkTradeReceivers(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(TradeReceivers)
	}
}

func BenchmarkTradeReceiverStrings(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(TradeReceiverStrings)
	}
}

func BenchmarkBookReceivers(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(BookReceivers)
	}
}

func BenchmarkLevel3Receivers(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(Level3Receivers)
	}
}

func BenchmarkLevel3ReceiverStrings(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(Level3ReceiverStrings)
	}
}

func BenchmarkAcceptedBookReceivers(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = len(AcceptedBookReceivers)
	}
}
