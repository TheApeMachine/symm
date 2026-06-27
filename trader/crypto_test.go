package trader

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

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

	done := make(chan error, 1)
	go func() {
		done <- crypto.Run()
	}()

	deadline := time.After(4 * time.Second)
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()

	for {
		select {
		case <-poll.C:
			if count := crypto.tick.Load(); count >= 25 {
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

func TestDecisionsArtifactPublishesAuthoritativeEmptyBatch(t *testing.T) {
	artifact := decisionsArtifact(
		nil,
		map[*datura.Artifact]struct{}{},
		map[*datura.Artifact]string{},
		time.Unix(0, 0).UTC(),
		42,
	)

	role, _ := artifact.Role()
	if role != "decisions" {
		t.Fatalf("role=%q, want decisions", role)
	}

	var payload struct {
		Seq       int64            `json:"seq"`
		Decisions []map[string]any `json:"decisions"`
	}

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		t.Fatalf("decode decisions payload: %v", err)
	}

	if payload.Seq != 42 {
		t.Fatalf("seq=%d, want 42", payload.Seq)
	}

	if payload.Decisions == nil || len(payload.Decisions) != 0 {
		t.Fatalf("empty candidate batch must publish empty decisions, got %#v", payload.Decisions)
	}
}

func TestDecisionsArtifactCarriesCandidateVerdicts(t *testing.T) {
	action := candidate("BTC/USD", logic.SideBuy, logic.ActionMarket, 0.72)
	chosen := map[*datura.Artifact]struct{}{action: {}}

	artifact := decisionsArtifact(
		[]*datura.Artifact{action},
		chosen,
		map[*datura.Artifact]string{},
		time.Unix(0, 0).UTC(),
		43,
	)

	var payload struct {
		Decisions []struct {
			Symbol     string  `json:"symbol"`
			Confidence float64 `json:"confidence"`
			Verdict    string  `json:"verdict"`
			Why        string  `json:"why"`
		} `json:"decisions"`
	}

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		t.Fatalf("decode decisions payload: %v", err)
	}

	if len(payload.Decisions) != 1 {
		t.Fatalf("decisions=%d, want 1", len(payload.Decisions))
	}

	decision := payload.Decisions[0]
	if decision.Symbol != "BTC/USD" || decision.Confidence != 0.72 ||
		decision.Verdict != "allow" || decision.Why != "admitted" {
		t.Fatalf("unexpected decision payload: %+v", decision)
	}
}
