package tables_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestScanOrdersBySeqIdx(t *testing.T) {
	Convey("Catalog.Scan yields rows sorted by SeqIdx", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog, 50)
		for _, seq := range []int64{3, 1, 2} {
			m := data.NewMeasurement("signal", map[string]data.Metric[float64]{"v": {Raw: float64(seq)}})
			m.Label = "BTC/USD"
			m.SeqIdx = seq
			m.At = time.Unix(seq, 0)
			writer.Add("measurements", data.Publication{Measurement: m})
		}
		So(writer.CommitReady(t.Context(), true), ShouldBeNil)

		got := make([]int64, 0, 3)
		for m := range catalog.Scan(t.Context(), tables.Measurements, 50, nil, 0) {
			got = append(got, m.SeqIdx)
		}
		So(got, ShouldResemble, []int64{1, 2, 3})
	})
}
