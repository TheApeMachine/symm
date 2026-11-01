package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/system"
)

func settled(book string, retPct float64) *broker.Position {
	return &broker.Position{
		Book:        book,
		Virtual:     book == "virtual",
		RealizedPct: retPct,
		PnLPct:      decimal.NewFromFloat64(retPct * 100),
	}
}

func TestConfidenceArmsAndDisarms(t *testing.T) {
	Convey("Confidence arms after confirm_after wins and disarms on demotion", t, func() {
		prev := system.Cfg
		system.Cfg = &system.Config{
			Learning: &system.Learning{
				ConfirmAfter:   3,
				DemotionLosses: 2,
				MinWinRate:     0.55,
				MinMeanReturn:  0.0,
			},
			Market: &system.Market{Model: "real"},
		}
		defer func() { system.Cfg = prev }()

		confidence := &Confidence{}

		So(confidence.Armed(), ShouldBeFalse)

		confidence.Record(settled("paper", 0.01))
		confidence.Record(settled("paper", 0.02))
		So(confidence.Armed(), ShouldBeFalse)

		confidence.Record(settled("paper", 0.01))
		So(confidence.Armed(), ShouldBeTrue)

		confidence.Record(settled("paper", -0.02))
		confidence.Record(settled("paper", -0.01))
		So(confidence.Armed(), ShouldBeFalse)

		snap := confidence.Snapshot()
		So(snap.Paper.Closed, ShouldEqual, 5)
		So(snap.Real.Halted, ShouldBeTrue)
		So(snap.RealCeiling, ShouldBeTrue)
	})
}

func TestConfidenceIgnoresVirtual(t *testing.T) {
	Convey("Virtual settles never count toward edge", t, func() {
		prev := system.Cfg
		system.Cfg = &system.Config{
			Learning: &system.Learning{
				ConfirmAfter:  1,
				MinWinRate:    0.5,
				MinMeanReturn: 0.0,
			},
			Market: &system.Market{Model: "real"},
		}
		defer func() { system.Cfg = prev }()

		confidence := &Confidence{}
		confidence.Record(settled("virtual", 0.5))
		So(confidence.Armed(), ShouldBeFalse)
		So(confidence.Snapshot().Paper.Closed, ShouldEqual, 0)
	})
}

func TestConfidencePaperCeilingBlocksReal(t *testing.T) {
	Convey("Paper market model never arms real", t, func() {
		prev := system.Cfg
		system.Cfg = &system.Config{
			Learning: &system.Learning{
				ConfirmAfter:  1,
				MinWinRate:    0.5,
				MinMeanReturn: 0.0,
			},
			Market: &system.Market{Model: "paper"},
		}
		defer func() { system.Cfg = prev }()

		confidence := &Confidence{}
		confidence.Record(settled("paper", 0.1))
		So(confidence.Armed(), ShouldBeFalse)
		So(confidence.Snapshot().Real.Halted, ShouldBeFalse)
		So(confidence.Snapshot().RealCeiling, ShouldBeFalse)
	})
}
