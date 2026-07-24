package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
)

func TestHubWriteWallet(t *testing.T) {
	previous := viper.Get("market.quote_currency")
	t.Cleanup(func() { viper.Set("market.quote_currency", previous) })

	Convey("Given a hub with a seeded balance", t, func() {
		viper.Set("market.quote_currency", "USD")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		messages := make(chan []byte, 4)
		balance := broker.NewBalance(nil, nil, messages, config.Fixture().Market)
		hub := NewHub(ctx, nil, balance, messages, config.UIConfig{Addr: "127.0.0.1:0"})
		defer hub.Close()

		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":250}]
		}`))

		Convey("writeWallet retains a fresh balances frame", func() {
			hub.writeWallet(nil)

			deadline := time.Now().Add(time.Second)

			for time.Now().Before(deadline) {
				payload := hub.Cached("balances")

				if len(payload) > 0 {
					So(string(payload), ShouldContainSubstring, `"USD"`)
					So(string(payload), ShouldContainSubstring, `250`)
					return
				}

				time.Sleep(5 * time.Millisecond)
			}

			So("retained", ShouldEqual, "missing")
		})
	})
}
