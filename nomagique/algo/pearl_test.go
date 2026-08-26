package algo

import (
	"errors"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmcausal "github.com/theapemachine/symm/nomagique/causal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestPearlDegenerateWindow(t *testing.T) {
	Convey("Given a retained window whose structural design is singular", t, func() {
		// Control 0 is exactly collinear with the treatment, so the
		// association and backdoor residualization stages pass while the
		// do-expectation structural fit [1, control0, control1, treatment]
		// is rank-deficient.
		rows := [][]float64{
			{1, 0, 1, 2},
			{2, 1, 2, 4},
			{3, 0, 3, 3},
			{4, 1, 4, 5},
		}

		frame := nmtypes.Frame{}
		frame.Put(nmcausal.SymbolRowCount, float64(len(rows)))
		frame.Put(nmcausal.SymbolTarget, 3)
		frame.Put(nmcausal.SymbolTreatment, 2)
		frame.Put(nmcausal.SymbolLevel, 2)
		frame.Put(nmcausal.SymbolBandwidth, 1)

		sampleIndex := 0

		for _, row := range rows {
			for _, value := range row {
				frame.Put(nmtypes.MustSampleSymbol(sampleIndex), value)
				sampleIndex++
			}
		}

		Convey("Pearl resolves to the non-identifiable state instead of a fatal error", func() {
			output := Pearl()(frame)
			So(errors.Is(output.Err, io.EOF), ShouldBeTrue)
		})
	})
}
