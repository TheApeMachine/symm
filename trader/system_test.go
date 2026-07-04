package trader

import (
	"context"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/logic"
)

func TestCryptoPlaybookDecisionDispatchesFixtureBackedPaperFill(t *testing.T) {
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

	balances := currentCryptoBalances(crypto)
	if balances == nil {
		t.Fatal("balances unavailable after private subscription")
	}
	defer balances.Release()

	measurements := []*datura.Artifact{
		playbookMeasurement(
			t,
			logic.SourcePumpDump,
			logic.CategoryVerticalIgnition,
			"ALGO/USD",
			0.82,
		),
		playbookMeasurement(
			t,
			logic.SourceHawkes,
			logic.CategoryFrenzy,
			"ALGO/USD",
			0.76,
		),
	}
	defer func() {
		for _, measurement := range measurements {
			measurement.Release()
		}
	}()

	crypto.story.Update(measurements)
	actions := crypto.story.Actions(balances)
	defer func() {
		for _, action := range actions {
			action.Release()
		}
	}()

	if len(actions) == 0 {
		t.Fatal("default playbook did not emit a clean-ignition candidate")
	}

	chosen, verdicts := crypto.decider.choose(measurements, actions, balances)
	if len(chosen) == 0 {
		t.Fatalf("decider rejected clean-ignition candidate: %#v", verdicts)
	}

	allowed := crypto.allocator.Allowed(chosen, balances)
	if len(allowed) == 0 {
		t.Fatal("allocator did not admit the chosen candidate")
	}

	if verdict := datura.Peek[string](allowed[0], "verdict"); verdict != "allow" {
		t.Fatalf("candidate verdict = %q, want allow", verdict)
	}
	if score := datura.Peek[float64](allowed[0], "decision", "score"); score <= 0 {
		t.Fatalf("candidate decision score = %v, want positive", score)
	}
	if fraction := datura.Peek[float64](allowed[0], "fraction"); fraction <= 0 {
		t.Fatalf("candidate fraction = %v, want positive", fraction)
	}

	if err := crypto.dispatch(allowed); err != nil {
		t.Fatalf("dispatch admitted playbook candidate: %v", err)
	}

	execution := waitForFilledExecution(t, executions, "ALGO/USD")
	if status := datura.Peek[string](execution, "data", 0, "order_status"); status != "filled" {
		t.Fatalf("order_status = %q, want filled", status)
	}

	waitForCryptoAssetAbove(t, crypto, "ALGO", 0)
	waitForFlowStats(t, crypto, 1, 1)
}

func playbookMeasurement(
	t *testing.T,
	source logic.SourceType,
	category logic.CategoryType,
	symbol string,
	confidence float64,
) *datura.Artifact {
	t.Helper()

	measurement := datura.Acquire("measurement", datura.APPJSON).
		WithRole("measurement").
		WithScope(symbol)
	if err := measurement.SetOrigin(string(source)); err != nil {
		t.Fatalf("set measurement origin: %v", err)
	}

	measurement.SetTimestamp(time.Now().UTC().UnixNano())
	measurement.MergeOutput("value", float64(logic.CategoryIndex(category)))
	measurement.MergeOutput("category", float64(logic.CategoryIndex(category)))
	measurement.MergeOutput("confidence", confidence)
	measurement.MergeOutput("strength", confidence)
	measurement.MergeOutput("surprise", 0.1)
	measurement.MergeOutput("entry_baseline", 0.25)
	measurement.MergeOutput("exit_baseline", 0.25)

	return measurement
}
