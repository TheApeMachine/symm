package trader

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

type cryptoTestTokenRest struct{}

func (cryptoTestTokenRest) WebSocketToken(_ context.Context, token *public.WebSocketToken) error {
	token.Token = "paper-test-token"
	token.Expires = 900
	return nil
}

func TestCryptoRunTicksPastFrontendFreezeRange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, &qpool.Config{
		SchedulingTimeout:  time.Second,
		JobChannelCapacity: 4,
		Scaler:             nil,
	})
	defer pool.Close()

	crypto, err := NewCrypto(ctx, pool, dmt.NewTree(""))
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()
	ui := pool.Subscribe("ui", nil)

	for balances := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
		balances.WithDestination("trader")
		if err := pool.CreateBroadcastGroup("balances").Send(balances); err != nil {
			t.Fatalf("send balances: %v", err)
		}
		break
	}

	done := make(chan error, 1)
	go func() {
		done <- crypto.Run()
	}()

	deadline := time.After(4 * time.Second)
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	sawTickFrame := false

	for {
		select {
		case <-poll.C:
			for artifact := ui.Poll(); artifact != nil; artifact = ui.Poll() {
				if role, _ := artifact.Role(); role != "tick" {
					continue
				}

				if count := datura.Peek[float64](artifact, "count"); count <= 0 {
					t.Fatalf("tick frame count = %v, want positive", count)
				}

				sawTickFrame = true
			}

			if count := crypto.tick.Load(); count >= 25 {
				if !sawTickFrame {
					t.Fatal("trader did not publish a tick frame with count")
				}

				cancel()
				if err := <-done; err != nil {
					t.Fatalf("crypto run returned error: %v", err)
				}
				return
			}
		case err := <-done:
			t.Fatalf("crypto run stopped before tick 25: %v", err)
		case <-deadline:
			t.Fatalf("crypto run reached tick %d, want at least 25", crypto.tick.Load())
		}
	}
}

func TestCryptoOnMessageKeepsLastBalancesWhenLocalFrameHasNoData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, &qpool.Config{
		SchedulingTimeout:  time.Second,
		JobChannelCapacity: 4,
		Scaler:             nil,
	})
	defer pool.Close()

	crypto, err := NewCrypto(ctx, pool, dmt.NewTree(""))
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()

	var good *datura.Artifact
	for balances := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
		good = balances
		break
	}

	if good == nil {
		t.Fatal("balances fixture did not yield")
	}

	if err := crypto.onMessage(good); err != nil {
		t.Fatalf("valid balances rejected: %v", err)
	}

	empty := datura.Acquire("test", datura.APPJSON).WithRole("balances")
	if err := crypto.onMessage(empty); err == nil {
		t.Fatal("empty balances artifact was accepted")
	}

	if crypto.Balances() != good {
		t.Fatal("empty balances artifact replaced last good balances")
	}
}

func TestCryptoPaperPrivateBalancesThroughWebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	previousModel := viper.GetString("trading.model")
	previousQuote := viper.GetString("market.quote_currency")
	previousWallet := viper.GetFloat64("trading.paper.wallet.usd")
	previousAddr := viper.GetString("emulator.addr")
	previousMaxDelay := viper.GetInt("system.network.connection.max_delay")
	previousLatency := viper.GetString("trading.paper.latency_profile")
	latencyPath := filepath.Join(t.TempDir(), "latency.json")

	if err := os.WriteFile(latencyPath, []byte(`{"latencies":[1]}`), 0o600); err != nil {
		t.Fatalf("write latency profile: %v", err)
	}

	viper.Set("trading.model", "paper")
	viper.Set("market.quote_currency", "USD")
	viper.Set("trading.paper.wallet.usd", 200.0)
	viper.Set("emulator.addr", freeCryptoTestListenAddr(t))
	viper.Set("system.network.connection.max_delay", 2)
	viper.Set("trading.paper.latency_profile", latencyPath)

	defer viper.Set("trading.model", previousModel)
	defer viper.Set("market.quote_currency", previousQuote)
	defer viper.Set("trading.paper.wallet.usd", previousWallet)
	defer viper.Set("emulator.addr", previousAddr)
	defer viper.Set("system.network.connection.max_delay", previousMaxDelay)
	defer viper.Set("trading.paper.latency_profile", previousLatency)

	public.BindTokenRest(cryptoTestTokenRest{})

	pool := qpool.NewQ[any](ctx, 1, 2, &qpool.Config{
		SchedulingTimeout:  time.Second,
		JobChannelCapacity: 4,
		Scaler:             nil,
	})
	defer pool.Close()

	tree := dmt.NewTree("")
	emulator, err := public.NewEmulator(ctx, pool, tree)
	if err != nil {
		t.Fatalf("new emulator: %v", err)
	}
	defer emulator.Close()

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
	defer accountSocket.Close()

	go accountSocket.Run(emulator.Endpoint())
	time.Sleep(100 * time.Millisecond)

	crypto, err := NewCrypto(ctx, pool, tree)
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()

	private := pool.CreateBroadcastGroup("kraken:private")
	deadline := time.Now().Add(4 * time.Second)

	for time.Now().Before(deadline) {
		subscribe := datura.Acquire("hub", datura.APPJSON).
			WithDestination("kraken:private").
			WithRole("balances").
			WithScope("subscribe").
			WithPayload(datura.Map[any]{
				"method": "subscribe",
				"params": datura.Map[any]{
					"channel": "balances",
				},
			}.Marshal())

		if err := private.Send(subscribe); err != nil {
			t.Fatalf("send private subscribe: %v", err)
		}

		balances := crypto.Balances()
		if balances != nil && datura.Peek[float64](balances, "data", 0, "balance") == 200.0 {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("paper private websocket did not deliver balances data: %v", crypto.Balances())
}

func freeCryptoTestListenAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	return listener.Addr().String()
}
