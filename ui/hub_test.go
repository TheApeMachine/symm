package ui

import (
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
