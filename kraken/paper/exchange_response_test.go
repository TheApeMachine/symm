package paper

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestWebSocketDefersExchangeResponses(t *testing.T) {
	Convey("Given measured exchange round-trip latency", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		profilePath := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", profilePath)

		defer viper.Set("trading.paper.latency_profile", "")

		pool := qpool.NewQ(ctx, 1, 4, nil)
		latency := public.SharedNetworkLatency()
		latency.RecordRTT(80 * time.Millisecond)

		trading.ResetDeskReady()
		t.Cleanup(func() {
			public.SharedNetworkLatency().Reset()
			trading.ResetDeskReady()
		})

		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		t.Cleanup(viper.Reset)

		_ = NewWebSocket(ctx, pool)

		Convey("It should not mark the desk ready before RTT elapses", func() {
			time.Sleep(20 * time.Millisecond)
			So(trading.DeskReady(), ShouldBeFalse)

			select {
			case <-time.After(120 * time.Millisecond):
				So(trading.DeskReady(), ShouldBeTrue)
			}
		})
	})
}
