package tests

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

/*
SessionEnv owns paper-harness process configuration: viper defaults for a
Session and Crypto wiring against temporary data paths.
*/
type SessionEnv struct{}

/*
NewSessionEnv returns the harness environment helper used by NewSession.
*/
func NewSessionEnv() SessionEnv {
	return SessionEnv{}
}

/*
Configure installs paper Session viper defaults and returns a restore func.
*/
func (env SessionEnv) Configure(testingTB testing.TB) func() {
	testingTB.Helper()

	previousModel := viper.Get("trading.model")
	previousData := viper.Get("system.data_path")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	previousSlots := viper.Get("trading.slots.normal")
	previousReserved := viper.Get("trading.slots.reserved")
	previousQuote := viper.Get("market.quote_currency")
	previousBatch := viper.Get("market.subscribe_batch")
	previousPace := viper.Get("market.subscribe_pace")
	previousFraction := viper.Get("trading.allocation.max_fraction")
	previousInterval := viper.Get("signals.fluid.integration_interval")

	viper.Set("trading.model", "paper")
	viper.Set("system.data_path", testingTB.TempDir())
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	viper.Set("trading.slots.normal", 2)
	viper.Set("trading.slots.reserved", 2)
	viper.Set("trading.allocation.max_fraction", 0.20)
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.subscribe_batch", 200)
	viper.Set("market.subscribe_pace", "20ms")
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	return func() {
		viper.Set("trading.model", previousModel)
		viper.Set("system.data_path", previousData)
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
		viper.Set("trading.slots.normal", previousSlots)
		viper.Set("trading.slots.reserved", previousReserved)
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.subscribe_pace", previousPace)
		viper.Set("trading.allocation.max_fraction", previousFraction)
		viper.Set("signals.fluid.integration_interval", previousInterval)
	}
}

/*
Crypto wires trader.Crypto for a Session against the paper stack.
*/
func (env SessionEnv) Crypto(
	ctx context.Context,
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
	desk *broker.Desk,
	instrument *broker.Instrument,
	planner *strategy.Planner,
	tree *dmt.Tree,
	channel chan []byte,
) (*trader.Crypto, error) {
	return trader.NewCrypto(
		ctx,
		system.NewBooter(ctx, channel),
		api,
		price,
		balance,
		desk,
		instrument,
		nil,
		planner,
		tree,
		types.NewThesis(channel, nil),
		nil,
		nil,
	)
}
