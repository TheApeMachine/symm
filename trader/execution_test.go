package trader

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/public/response"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
	tickerfixtures "github.com/theapemachine/symm/tests/fixtures/ticker"
)

func TestCryptoPaperTradeFillsAndUpdatesDeskFromFixtures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wallet := fixtureUSDBalance(t)
	configurePaperExecutionTest(t, wallet)
	public.BindTokenRest(cryptoTestTokenRest{})

	pool := qpool.NewQ[any](ctx, 1, 2, &qpool.Config{
		SchedulingTimeout:  time.Second,
		JobChannelCapacity: 8,
		Scaler:             nil,
	})
	defer pool.Close()

	tree := dmt.NewTree("")
	emulator, err := public.NewEmulator(ctx, pool, tree)
	if err != nil {
		t.Fatalf("new emulator: %v", err)
	}

	go func() {
		_ = emulator.Serve()
	}()

	accountSocket := public.NewWebSocket(
		ctx,
		pool,
		tree,
		websocket.DefaultDialer,
		[]string{"balances", "executions", "orders"},
		[]string{"kraken:private"},
	)
	defer func() {
		cancel()
		_ = accountSocket.Close()
		_ = emulator.Close()
	}()

	go accountSocket.Run(emulator.Endpoint())
	time.Sleep(100 * time.Millisecond)

	crypto, err := NewCrypto(ctx, pool, tree)
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()

	executions := pool.Subscribe("executions", nil)
	subscribePrivateBalances(t, pool)
	waitForCryptoAsset(t, crypto, "USD", wallet)
	publishTickerFixture(t, pool, tree)

	action := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("ALGO/USD").
		WithPayload(datura.Map[any]{
			"type":     "market",
			"side":     "buy",
			"fraction": 0.05,
		}.Marshal()).
		Poke(true, "allowed").
		Poke(true, "risk", "stamped").
		Poke(0.05, "fraction")
	defer action.Release()

	if err := crypto.dispatch([]*datura.Artifact{action}); err != nil {
		t.Fatalf("dispatch fixture-backed action: %v", err)
	}

	execution := waitForFilledExecution(t, executions, "ALGO/USD")
	if status := datura.Peek[string](execution, "data", 0, "order_status"); status != "filled" {
		t.Fatalf("order_status = %q, want filled", status)
	}
	if price := datura.Peek[float64](execution, "data", 0, "last_price"); price <= 0 {
		t.Fatalf("last_price = %v, want positive fill", price)
	}

	waitForCryptoAssetAbove(t, crypto, "ALGO", 0)
	waitForFlowStats(t, crypto, 1, 1)
}

func configurePaperExecutionTest(t *testing.T, wallet float64) {
	t.Helper()

	latencyPath := t.TempDir() + "/latency.json"
	if err := os.WriteFile(latencyPath, []byte(`{"latencies":[1]}`), 0o600); err != nil {
		t.Fatalf("write latency profile: %v", err)
	}

	values := map[string]any{
		"trading.model":                       viper.GetString("trading.model"),
		"market.quote_currency":               viper.GetString("market.quote_currency"),
		"trading.paper.wallet.usd":            viper.GetFloat64("trading.paper.wallet.usd"),
		"emulator.addr":                       viper.GetString("emulator.addr"),
		"system.network.connection.max_delay": viper.GetInt("system.network.connection.max_delay"),
		"trading.paper.latency_profile":       viper.GetString("trading.paper.latency_profile"),
		"trading.paper.taker_fee_bps":         viper.GetFloat64("trading.paper.taker_fee_bps"),
		"trading.paper.maker_fee_bps":         viper.GetFloat64("trading.paper.maker_fee_bps"),
		"trading.max_quote_age":               viper.GetDuration("trading.max_quote_age"),
		"trading.max_spread_bps":              viper.GetFloat64("trading.max_spread_bps"),
		"trading.max_slippage_bps":            viper.GetFloat64("trading.max_slippage_bps"),
		"trading.replay.min_depth_coverage":   viper.GetFloat64("trading.replay.min_depth_coverage"),
		"trading.paper.rate_limits.enabled":   viper.GetBool("trading.paper.rate_limits.enabled"),
		"trading.sizing.base_fraction":        viper.GetFloat64("trading.sizing.base_fraction"),
	}

	t.Cleanup(func() {
		for key, value := range values {
			viper.Set(key, value)
		}
	})

	viper.Set("trading.model", "paper")
	viper.Set("market.quote_currency", "USD")
	viper.Set("trading.paper.wallet.usd", wallet)
	viper.Set("emulator.addr", freeCryptoTestListenAddr(t))
	viper.Set("system.network.connection.max_delay", 2)
	viper.Set("trading.paper.latency_profile", latencyPath)
	viper.Set("trading.paper.taker_fee_bps", 40.0)
	viper.Set("trading.paper.maker_fee_bps", 25.0)
	viper.Set("trading.max_quote_age", 0)
	viper.Set("trading.max_spread_bps", 0.0)
	viper.Set("trading.max_slippage_bps", 0.0)
	viper.Set("trading.replay.min_depth_coverage", 0.0)
	viper.Set("trading.paper.rate_limits.enabled", false)
	viper.Set("trading.sizing.base_fraction", 0.05)
}

func fixtureUSDBalance(t *testing.T) float64 {
	t.Helper()

	for balances := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
		for rowIndex := 0; ; rowIndex++ {
			asset := datura.Peek[string](balances, "data", rowIndex, "asset")
			if asset == "" {
				break
			}
			if asset == "USD" {
				return datura.Peek[float64](balances, "data", rowIndex, "balance")
			}
		}
	}

	t.Fatal("balance fixture did not contain USD")
	return 0
}

func subscribePrivateBalances(t *testing.T, pool *qpool.Q[any]) {
	t.Helper()

	subscribe := datura.Acquire("test", datura.APPJSON).
		WithDestination("kraken:private").
		WithRole("balances").
		WithScope("subscribe").
		WithPayload(datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "balances",
			},
		}.Marshal())
	defer subscribe.Release()

	if err := pool.CreateBroadcastGroup("kraken:private").Send(subscribe); err != nil {
		t.Fatalf("send private balance subscribe: %v", err)
	}
}

func publishTickerFixture(t *testing.T, pool *qpool.Q[any], tree *dmt.Tree) {
	t.Helper()

	handler := response.NewTreeHandler(tree)
	for ticker := range tickerfixtures.NewFixture(tickerfixtures.UPDATE, 1).Artifacts() {
		_ = ticker.SetOrigin("kraken:public")
		ticker.SetTimestamp(time.Now().UTC().UnixNano())
		handler.Send(ticker)
		ticker.WithDestination("broker")
		if err := pool.CreateBroadcastGroup("ticker").Send(ticker); err != nil {
			t.Fatalf("send ticker fixture: %v", err)
		}
		return
	}

	t.Fatal("ticker fixture did not yield")
}

func waitForFilledExecution(
	t *testing.T,
	executions *qpool.BroadcastConsumer,
	symbol string,
) *datura.Artifact {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		execution, err := executions.Wait(ctx)
		if err != nil {
			t.Fatalf("wait for execution: %v", err)
		}

		if datura.Peek[string](execution, "data", 0, "symbol") != symbol {
			continue
		}

		if datura.Peek[string](execution, "data", 0, "order_status") == "filled" {
			return execution
		}
	}
}

func waitForCryptoAsset(
	t *testing.T,
	crypto *Crypto,
	asset string,
	expected float64,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if balance := cryptoAssetBalance(crypto, asset); balance == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("asset %s balance did not reach %v", asset, expected)
}

func waitForCryptoAssetAbove(
	t *testing.T,
	crypto *Crypto,
	asset string,
	minimum float64,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if balance := cryptoAssetBalance(crypto, asset); balance > minimum {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("asset %s balance did not rise above %v", asset, minimum)
}

func cryptoAssetBalance(crypto *Crypto, asset string) float64 {
	balances := currentCryptoBalances(crypto)
	if balances == nil {
		return 0
	}

	for rowIndex := 0; ; rowIndex++ {
		current := datura.Peek[string](balances, "data", rowIndex, "asset")
		if current == "" {
			return 0
		}
		if current == asset {
			return datura.Peek[float64](balances, "data", rowIndex, "balance")
		}
	}
}

func waitForFlowStats(
	t *testing.T,
	crypto *Crypto,
	submitted int,
	filled int,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stats := crypto.desk.FlowStats()
		if stats.SubmittedCount == submitted && stats.FilledCount == filled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	stats := crypto.desk.FlowStats()
	t.Fatalf(
		"flow stats submitted/filled = %d/%d, want %d/%d",
		stats.SubmittedCount,
		stats.FilledCount,
		submitted,
		filled,
	)
}
