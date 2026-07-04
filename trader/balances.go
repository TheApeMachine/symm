package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
BalanceSnapshot is the immutable balance frame Crypto reads on the tick path.
*/
type BalanceSnapshot struct {
	origin       string
	scope        string
	artifactType datura.Artifact_Type
	timestamp    int64
	payload      []byte
}

func NewBalanceSnapshot(artifact *datura.Artifact) (*BalanceSnapshot, error) {
	if artifact == nil {
		return nil, errnie.Err(errnie.Validation, "trader: balances artifact is nil", nil)
	}

	if len(datura.Peek[[]any](artifact, "data")) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"trader: balances artifact missing data",
			nil,
		).With(artifact.Log()...)
	}

	origin := datura.Peek[string](artifact, "origin")
	scope := datura.Peek[string](artifact, "scope")
	payload := append([]byte(nil), artifact.DecryptPayload()...)

	if origin == "" || scope == "" || len(payload) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"trader: balances artifact incomplete",
			nil,
		).With(artifact.Log()...)
	}

	artifactType := artifact.Type()
	if artifactType == 0 {
		artifactType = datura.APPJSON
	}

	snapshot := &BalanceSnapshot{
		origin:       origin,
		scope:        scope,
		artifactType: artifactType,
		timestamp:    artifact.Timestamp(),
		payload:      payload,
	}

	if _, err := snapshot.Artifact(); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (snapshot *BalanceSnapshot) Artifact() (*datura.Artifact, error) {
	if snapshot == nil {
		return nil, errnie.Err(errnie.Validation, "trader: balances snapshot is nil", nil)
	}

	if snapshot.origin == "" || snapshot.scope == "" || len(snapshot.payload) == 0 {
		return nil, errnie.Err(errnie.Validation, "trader: balances snapshot incomplete", nil)
	}

	artifactType := snapshot.artifactType
	if artifactType == 0 {
		artifactType = datura.APPJSON
	}

	balances := datura.Acquire(snapshot.origin, artifactType).
		WithRole("balances").
		WithScope(snapshot.scope).
		WithPayload(append([]byte(nil), snapshot.payload...))
	balances.SetTimestamp(snapshot.timestamp)

	if len(datura.Peek[[]any](balances, "data")) == 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"trader: balances snapshot copy missing data",
			nil,
		).With(balances.Log()...)
	}

	return balances, nil
}

func (crypto *Crypto) balanceArtifact() (*datura.Artifact, error) {
	snapshot := crypto.balances.Load()

	if snapshot == nil {
		return nil, errnie.Err(errnie.Validation, "trader: balances artifact unavailable", nil)
	}

	return snapshot.Artifact()
}
