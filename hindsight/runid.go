package hindsight

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
)

/*
RunIdentity is the pre-capture metadata that distinguishes one process capture
session (§5). It carries the process start instant, the code commit, the build
identity, a digested configuration identity, and the schema versions the run
understands — enough context to make a captured Run interpretable, and to make
two runs distinguishable even when they start in the same instant.
*/
type RunIdentity struct {
	StartedAt      time.Time
	CodeCommit     string
	BuildID        string
	ConfigDigest   string
	SchemaVersions map[string]string
}

/*
NewRunID derives a stable, collision-resistant RunID for a process run. It is
built from the run's process start time plus a random nonce, so two process
runs can never generate the same logical run identity — even two that begin
within the same wall-clock instant. The RunID is opaque and carries no storage
row identity (§50).
*/
func NewRunID(startedAt time.Time) (RunID, error) {
	if startedAt.IsZero() {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"hindsight: run identity requires a start instant",
			nil,
		))
	}

	nonce := make([]byte, 16)

	if _, err := rand.Read(nonce); err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"hindsight: generate run nonce",
			err,
		))
	}

	return RunID(fmt.Sprintf(
		"%d-%s",
		startedAt.UTC().UnixNano(),
		hex.EncodeToString(nonce),
	)), nil
}

/*
Resolve builds the Run record from this identity plus an ID. Two RunIdentity
values with the same Commit, BuildID, ConfigDigest, and schema versions are the
same code/config; the RunID (nonce + start instant) is what distinguishes the
process runs themselves.
*/
func (identity RunIdentity) Resolve(id RunID) Run {
	return Run{
		ID:             id,
		StartedAt:      identity.StartedAt,
		CodeCommit:     identity.CodeCommit,
		BuildID:        identity.BuildID,
		ConfigDigest:   identity.ConfigDigest,
		SchemaVersions: identity.SchemaVersions,
		Integrity:      IntegrityComplete,
	}
}
