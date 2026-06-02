package replay

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestScanWSRows(t *testing.T) {
	Convey("Given a JSONL capture on disk", t, func() {
		path := filepath.Join(t.TempDir(), "scan.jsonl")
		recorder, err := OpenRecorder(path)

		So(err, ShouldBeNil)

		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","price":1}]}`)
		So(WriteWS(public.TradesChannel, DirectionIn, payload), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		rows, err := ScanWSRows[map[string]any](ctx, path, public.TradesChannel)

		So(err, ShouldBeNil)

		row, ok := <-rows

		Convey("It should scan decoded rows for one channel", func() {
			So(ok, ShouldBeTrue)
			So((*row)["symbol"], ShouldEqual, "BTC/EUR")
		})
	})
}
