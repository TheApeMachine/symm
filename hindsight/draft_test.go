package hindsight

import (
	"testing"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

func draftValue(aTick float64) *data.Value {
	value := data.NewValue("detect", "up", "BTC/USD")
	value.Append([]float64{
		10, aTick, 20, aTick + 10, 40, aTick + 30, 50, aTick + 40, 1.1, 1.2,
	}, "a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick", "b_price", "c_price")

	return value
}

func TestWriteDraftRejectsMissingA(t *testing.T) {
	err := writeDraftTo(nil, draftValue(0))
	if err == nil {
		t.Fatal("a draft with no A was stored by copying B")
	}
}

func TestWriteDraftRejectsLeadWithoutTheShiftedPrint(t *testing.T) {
	system.Cfg = system.NewConfig()
	system.Cfg.Detector = &system.Detector{ExecutionLeadFraction: 0.1}

	err := writeDraftTo(nil, draftValue(1000))
	if err == nil {
		t.Fatal("lead moved B/C and the original price was kept")
	}
}
