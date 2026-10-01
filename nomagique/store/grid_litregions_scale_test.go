package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLitRegionsCommensurateActivity(t *testing.T) {
	Convey("top-N uses commensurate activity so price raw cannot own every slot", t, func() {
		grid := NewGrid()
		grid.Regions = map[string]uint8{
			cellKey("BTC/USD", "", "mid"):      10,
			cellKey("BTC/USD", "", "spread"):   20,
			cellKey("BTC/USD", "", "contrast"): 30,
			cellKey("BTC/USD", "", "flow"):     40,
		}

		nContrast := 0.9
		nFlow := 0.85
		nMid := 0.2
		nSpread := 0.15
		frame := &data.Measurement[float64]{Label: "BTC/USD"}
		frame.SetMetric("mid", data.Metric[float64]{Label: "mid", Raw: 100000, Normalized: &nMid})
		frame.SetMetric("spread", data.Metric[float64]{Label: "spread", Raw: 12, Normalized: &nSpread})
		frame.SetMetric("contrast", data.Metric[float64]{Label: "contrast", Raw: 0.9, Normalized: &nContrast})
		frame.SetMetric("flow", data.Metric[float64]{Label: "flow", Raw: 0.7, Normalized: &nFlow})

		token := grid.LitRegions(frame)
		So(token, ShouldNotBeNil)
		So(len(token), ShouldEqual, litRegionTokenSize)

		seen := map[byte]bool{}
		for _, r := range token {
			seen[r] = true
		}

		// Top activity: contrast(0.9), flow(0.85), mid(0.2) — spread dropped.
		So(seen[30], ShouldBeTrue)
		So(seen[40], ShouldBeTrue)
		So(seen[10], ShouldBeTrue)
		So(seen[20], ShouldBeFalse)
		// Canonical ascending emit after top-N pick.
		So(token, ShouldResemble, []byte{10, 30, 40})
	})
}

func TestLitRegionsCanonicalOrder(t *testing.T) {
	Convey("same top-N membership encodes ascending region IDs", t, func() {
		grid := NewGrid()
		grid.Regions = map[string]uint8{
			cellKey("BTC/USD", "", "a"): 5,
			cellKey("BTC/USD", "", "b"): 9,
			cellKey("BTC/USD", "", "c"): 2,
		}

		eq := 1.0
		left := &data.Measurement[float64]{Label: "BTC/USD"}
		left.SetMetric("a", data.Metric[float64]{Label: "a", Raw: 1, Normalized: &eq})
		left.SetMetric("b", data.Metric[float64]{Label: "b", Raw: 1, Normalized: &eq})
		left.SetMetric("c", data.Metric[float64]{Label: "c", Raw: 1, Normalized: &eq})

		right := &data.Measurement[float64]{Label: "BTC/USD"}
		right.SetMetric("a", data.Metric[float64]{Label: "a", Raw: 1, Normalized: &eq})
		right.SetMetric("b", data.Metric[float64]{Label: "b", Raw: 1, Normalized: &eq})
		right.SetMetric("c", data.Metric[float64]{Label: "c", Raw: 1, Normalized: &eq})

		So(grid.LitRegions(left), ShouldResemble, grid.LitRegions(right))
		So(grid.LitRegions(left), ShouldResemble, []byte{2, 5, 9})
	})
}

func TestLitRegionsTopNNotMean(t *testing.T) {
	Convey("token length is N most-lit, not mean-threshold membership", t, func() {
		grid := NewGrid()
		grid.Regions = map[string]uint8{
			cellKey("BTC/USD", "", "a"): 1,
			cellKey("BTC/USD", "", "b"): 2,
			cellKey("BTC/USD", "", "c"): 3,
			cellKey("BTC/USD", "", "d"): 4,
			cellKey("BTC/USD", "", "e"): 5,
		}

		frame := &data.Measurement[float64]{Label: "BTC/USD"}
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			frame.SetMetric(name, data.Metric[float64]{Label: name, Raw: 1})
		}

		token := grid.LitRegions(frame)
		So(len(token), ShouldEqual, litRegionTokenSize)
	})
}
