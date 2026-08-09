package tests

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
WithAutoFill enables the configured stateful execution model.
*/
func (market *Market) WithAutoFill(options ...AutoFillOptions) *Market {
	config := market.Config.Execution

	if len(options) > 1 {
		panic("simulator: WithAutoFill accepts at most one execution configuration")
	}

	if len(options) > 0 {
		config = options[0]
	}

	if err := config.Validate(); err != nil {
		panic(err)
	}
	config.Outcomes = append([]testtypes.OrderOutcome(nil), config.Outcomes...)

	market.Config.Execution = config

	for _, generator := range market.generators {
		if err := generator.ConfigureDepth(
			config.DepthLevels,
			config.DepthQuantityScale,
		); err != nil {
			panic(err)
		}
	}

	market.execution = newExecutionModel(
		config, market.Config.Profiles,
		market.Symbols, market.Private, market.Config.Seed,
	)
	market.autoFill = true

	return market
}

func (market *Market) Close() {
	market.Public.Close()
	market.Private.Close()
	market.Level3.Close()
	market.cancel()
}

/*
WithFixtureOrders temporarily configures authenticated fixture order routing.
*/
func WithFixtureOrders(
	t *testing.T,
	symbols []*testtypes.Symbol,
	f func(*Market),
) func() {
	return func() {
		tradingModel := viper.GetString("trading.model")
		apiKey, hadAPIKey := os.LookupEnv("KRAKEN_API_KEY")
		apiSecret, hadAPISecret := os.LookupEnv("KRAKEN_API_SECRET")

		viper.Set("trading.model", "real")
		_ = os.Setenv("KRAKEN_API_KEY", "fixture-key")
		_ = os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")

		defer func() {
			viper.Set("trading.model", tradingModel)

			if hadAPIKey {
				_ = os.Setenv("KRAKEN_API_KEY", apiKey)
			} else {
				_ = os.Unsetenv("KRAKEN_API_KEY")
			}

			if hadAPISecret {
				_ = os.Setenv("KRAKEN_API_SECRET", apiSecret)
			} else {
				_ = os.Unsetenv("KRAKEN_API_SECRET")
			}
		}()

		WithMarket(t, symbols, f)()
	}
}

/*
WithMarket runs a default configured venue for one test.
*/
func WithMarket(t *testing.T, symbols []*testtypes.Symbol, f func(*Market)) func() {
	return WithScenario(t, testtypes.NewScenarioConfig(symbols), func(market *Market) {
		Reset(func() {
			market.Close()
		})

		f(market)
	})
}

/*
Feeds returns the public and private production websocket sessions.
*/
func (market *Market) Feeds() (*websocket.Live, *websocket.Live) {
	return market.public, market.private
}
