package trader

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

func TestCryptoManageTTL(t *testing.T) {
	convey.Convey("Given a held position past PerspectiveTTL", t, func() {
		crypto := newTestCrypto()
		symbol := "BTC/EUR"
		base := "BTC"
		now := time.Now()

		crypto.wallet.BindPosition(base, wallet.PositionBinding{
			Source:      "perspective",
			Playbook:    string(perspectives.PlaybookDrive),
			PredictedAt: now.Add(-2 * config.System.PerspectiveTTL),
			DueAt:       now.Add(-time.Second),
		})
		crypto.wallet.AddInventory(base, 0.01, 100)
		crypto.positions.Open(symbol, positionState{Playbook: string(perspectives.PlaybookDrive), Peak: 100})

		convey.Convey("It should exit on manage even without exit categories", func() {
			crypto.manage(symbol, 100, nil)
			_, held := crypto.wallet.PositionBindingFor(base)

			convey.So(held, convey.ShouldBeFalse)
		})
	})
}

func TestCryptoPumpTrail(t *testing.T) {
	convey.Convey("Given a pump position with a large retrace from peak", t, func() {
		crypto := newTestCrypto()
		symbol := "PUMP/EUR"

		crypto.wallet.BindPosition("PUMP", wallet.PositionBinding{
			Source:      "perspective",
			Playbook:    string(perspectives.PlaybookPump),
			PredictedAt: time.Now(),
			DueAt:       time.Now().Add(config.System.PerspectiveTTL),
		})
		crypto.wallet.AddInventory("PUMP", 0.01, 100)
		crypto.positions.Open(symbol, positionState{
			Playbook: string(perspectives.PlaybookPump),
			Peak:     100,
		})

		convey.Convey("It should exit before asking the tree", func() {
			last := 100 * (1 - config.System.PumpTrailPct - 0.01)
			crypto.manage(symbol, last, nil)
			_, held := crypto.wallet.PositionBindingFor("PUMP")

			convey.So(held, convey.ShouldBeFalse)
		})
	})
}
