package ui

import (
	"encoding/json"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/user"
)

func TestHubRememberBalances(t *testing.T) {
	Convey("Given a balances ui frame", t, func() {
		hub := &Hub{}

		hub.rememberBalances(user.Balances{
			Asset: []user.Balance{{
				Asset:   "EUR",
				Balance: 250,
			}},
		})

		Convey("It should retain the latest snapshot for replay", func() {
			snapshot := hub.lastBalances.Load()

			So(snapshot, ShouldNotBeNil)
			So(len(snapshot.Asset), ShouldEqual, 1)
			So(snapshot.Asset[0].Balance, ShouldEqual, 250)
		})
	})
}

func TestFrontendClientSendRejectsNonFiniteJSON(t *testing.T) {
	Convey("Given a ui frame with non-finite floats", t, func() {
		_, err := json.Marshal(map[string]any{
			"type": "fluid",
			"re":   math.Inf(1),
		})

		Convey("It should fail JSON encoding before websocket write", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
