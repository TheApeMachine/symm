package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoSendWallet(t *testing.T) {
	Convey("Given crypto with gauge confidence history", t, func() {
		crypto := newEnginePulseTestCrypto(t)
		crypto.gaugeAvg.Observe("hawkes", 0.42)
		subscriber := crypto.broadcasts["ui"].Subscribe("wallet-test", 8)

		crypto.sendWallet()

		Convey("It should publish gauge confidence with the wallet snapshot", func() {
			select {
			case value := <-subscriber.Incoming:
				frame, ok := value.Value.(map[string]any)
				So(ok, ShouldBeTrue)
				So(frame["event"], ShouldEqual, "wallet")

				gauges, ok := frame["gauge_confidence"].(map[string]float64)
				So(ok, ShouldBeTrue)
				So(gauges["hawkes"], ShouldAlmostEqual, 0.42)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for wallet frame")
			}
		})
	})
}
