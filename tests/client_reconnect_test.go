//go:build !race

package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
The API SDK v2.0.3 reconnects its next reader before the previous reader has
returned, and both write an unexported activity flag without synchronization.
The normal suite verifies the real SDK reconnect boundary; the race build omits
only this dependency-owned transition until the SDK synchronizes that flag.
*/
func TestConnReconnect(t *testing.T) {
	Convey("Given a Conn with an exact reconnect fault", t, func() {
		conn := NewConn(t.Context())
		defer conn.Close()
		conn.ConfigureFaults(testtypes.FaultConfig{Rules: []testtypes.FaultRule{{
			Channel: "ticker", Occurrence: 1, Action: testtypes.FaultReconnect,
		}}})
		So(conn.Connect(), ShouldBeNil)
		before := conn.connectionGeneration
		published := conn.Publish("ticker", []byte(`{"channel":"ticker"}`))

		Convey("The SDK should reconnect through the fixture dial boundary", func() {
			So(published, ShouldBeFalse)
			So(conn.connectionGeneration, ShouldBeGreaterThan, before)
			So(conn.faults.Report().Reconnects, ShouldEqual, 1)
		})
	})
}
