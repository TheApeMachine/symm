package frame

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

func TestPublishBalancesMatchesLivePrivateBus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")
	subscription := pool.Subscribe("ui", nil)
	ui := pool.CreateBroadcastGroup("ui")

	wire := []byte(`{"channel":"balances","type":"snapshot","data":[{"asset":"USD","asset_class":"currency","balance":200}]}`)
	message := types.Acquire()
	defer message.Release()

	if err := message.Decode(wire); err != nil {
		t.Fatal(err)
	}

	if err := Publish(tree, ui, wire, message); err != nil {
		t.Fatal(err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	artifact, err := subscription.Wait(waitCtx)

	if err != nil {
		t.Fatal(err)
	}

	role, roleErr := artifact.Role()
	scope, scopeErr := artifact.Scope()

	if roleErr != nil {
		t.Fatal(roleErr)
	}

	if scopeErr != nil {
		t.Fatal(scopeErr)
	}

	if role != "balances" || scope != "snapshot" {
		t.Fatalf("role/scope = %q/%q, want balances/snapshot", role, scope)
	}

	var payload map[string]any

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		t.Fatal(err)
	}

	rows, ok := payload["asset"].([]any)

	if !ok || len(rows) != 1 {
		t.Fatalf("asset rows = %#v, want one row", payload["asset"])
	}
}

func TestAckOnlyRequest(t *testing.T) {
	if !AckOnlyRequest([]byte(`{"method":"subscribe","params":{"channel":"orders"}}`)) {
		t.Fatal("subscribe should be ack-only")
	}

	if AckOnlyRequest([]byte(`{"method":"add_order","params":{"symbol":"BTC/USD"}}`)) {
		t.Fatal("add_order should not be ack-only")
	}
}

func BenchmarkPublishBalances(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")
	ui := pool.CreateBroadcastGroup("ui")
	wire := []byte(`{"channel":"balances","type":"update","data":[{"asset":"USD","balance":100}]}`)
	message := types.Acquire()
	defer message.Release()

	if err := message.Decode(wire); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := Publish(tree, ui, wire, message); err != nil {
			b.Fatal(err)
		}
	}
}
