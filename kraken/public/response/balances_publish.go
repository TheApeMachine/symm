package response

import (
	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/types"
)

func (balances *Balances) snapshot(scope string) *datura.Artifact {
	payload := balances.payload()
	payload["channel"] = "balances"
	payload["type"] = scope
	balances.storePayload(payload)

	out := datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"balances",
	).WithScope(
		scope,
	)
	out.SetTimestamp(balances.currentTime().UnixNano())
	out.WithPayload(balances.model.DecryptPayload())

	return out
}

func (balances *Balances) publish(artifact *datura.Artifact, internal bool) {
	if artifact == nil {
		return
	}

	balances.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	if !internal {
		return
	}

	if balances.tree != nil {
		balances.tree.InsertArtifact(artifact.Prefix(
			"role", "timestamp", "scope",
		), artifact)
	}

	if balances.pool != nil {
		artifact.WithDestination("balances")
		balances.pool.CreateBroadcastGroup("balances").Send(artifact)
		balances.pool.CreateBroadcastGroup("ui").Send(uiBalanceArtifact(artifact))
	}
}

func uiBalanceArtifact(artifact *datura.Artifact) *datura.Artifact {
	if artifact == nil {
		return nil
	}

	scope, _ := artifact.Scope()
	uiArtifact := datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"balances",
	).WithScope(
		scope,
	).WithDestination(
		"ui",
	).WithPayload(
		artifact.DecryptPayload(),
	)

	if timestamp := artifact.Timestamp(); timestamp > 0 {
		uiArtifact.SetTimestamp(timestamp)
	}

	return uiArtifact
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers.Store(uuid.NewString(), socket)
	}
}
