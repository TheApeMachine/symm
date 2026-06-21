package trader

import (
	"context"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/pumpdump"
)

func TestPumpdumpUIWireMatchesFrontendContract(testingTB *testing.T) {
	Convey("Given a pumpdump measurement published like crypto.Run", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		tree := dmt.NewTree(testingTB.TempDir())
		crypto := NewCrypto(ctx, pool, tree)

		defer func() {
			_ = crypto.Close()
		}()

		replayAt := time.Now().UnixNano()
		ingestProgressiveTicker(tree, 59, 100, 10000, &replayAt)
		ingestVerticalTicker(tree, &replayAt)

		signal := pumpdump.NewSignal(ctx, pool, tree)

		defer func() {
			_ = signal.Close()
		}()

		var measurement *datura.Artifact

		for stored := range tree.Seek([]byte("ticker/update")) {
			measurement = signal.Measure(stored)
		}

		So(measurement, ShouldNotBeNil)

		tagMeasurementForUI(measurement, logic.SourcePumpDump)
		measurement.WithDestination("ui")

		So(crypto.uiBroadcast.Send(measurement), ShouldBeNil)

		received, waitErr := subscription.Wait(ctx)

		So(waitErr, ShouldBeNil)
		So(received, ShouldNotBeNil)

		hubWire := make([]byte, 256*1024)
		hubCount, hubReadErr := received.Read(hubWire)

		So(hubReadErr, ShouldEqual, io.EOF)
		So(hubCount, ShouldBeGreaterThan, 0)

		decoded := datura.Acquire("ui-wire-test", datura.APPJSON)
		_, writeErr := decoded.Write(hubWire[:hubCount])

		So(writeErr, ShouldBeNil)

		role, _ := decoded.Role()
		origin, _ := decoded.Origin()
		scope, _ := decoded.Scope()
		destination, _ := decoded.Destination()

		So(role, ShouldEqual, "measurement")
		So(origin, ShouldEqual, string(logic.SourcePumpDump))
		So(scope, ShouldNotEqual, "update")
		So(destination, ShouldEqual, "ui")
		So(datura.Peek[float64](decoded, "output", "confidence"), ShouldBeGreaterThan, 0)
	})
}
