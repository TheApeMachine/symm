package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestSignalMeasureSeekPrefix(t *testing.T) {
	Convey("Given ingest keyed by timestamp like the websocket", t, func() {
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		at := time.Now().UTC().Truncate(time.Second)
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5}]}`))
		artifact.SetTimestamp(at.UnixNano())

		tree.Insert(artifact.Prefix("timestamp"), artifact.Pack())
		artifact.Release()

		measurements := runner.Measure()

		Convey("It should replay the ticker frame", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)
		})
	})
}
