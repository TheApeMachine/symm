package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

type sqliteExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

/*
writeOperations commits one ordered capture and manifest batch atomically. A
single transaction removes per-frame SQLite commits from transport callbacks
while preserving capture-before-manifest insertion order.
*/
func (store *SQLite) writeOperations(operations []writerOperation) error {
	if store == nil || store.database == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	transaction, err := store.database.Begin()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: begin capture batch",
			err,
		))
	}

	for _, operation := range operations {
		if err := store.writeOperation(transaction, operation); err != nil {
			_ = transaction.Rollback()

			return err
		}
	}

	if err := transaction.Commit(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: commit capture batch",
			err,
		))
	}

	return nil
}

func (store *SQLite) writeOperation(executor sqliteExecutor, operation writerOperation) error {
	if store == nil || store.database == nil || executor == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: sqlite database required",
			nil,
		))
	}

	switch operation.kind {
	case writerCapture:
		return store.writeCaptureOperation(executor, operation)
	case writerManifest:
		return store.writeManifestOperation(executor, operation.manifest)
	case writerLifecycle:
		return store.writeLifecycleOperation(
			executor,
			operation.runID,
			operation.lifecycle,
		)
	case writerFence:
		return nil
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("store: unknown writer operation %d", operation.kind),
			nil,
		))
	}
}

func (store *SQLite) writeLifecycleOperation(
	executor sqliteExecutor,
	runID hindsight.RunID,
	event hindsight.LifecycleEvent,
) error {
	if event.DecisionID == "" || event.Kind == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: lifecycle decision id and kind required",
			nil,
		))
	}

	executionJSON := ""

	if event.Execution != nil {
		encoded, err := json.Marshal(event.Execution)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"store: marshal lifecycle execution failed",
				err,
			))
		}

		executionJSON = string(encoded)
	}

	if _, err := executor.Exec(
		`INSERT INTO lifecycle (run_id, decision_id, symbol, kind, action, at, execution)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(runID),
		event.DecisionID,
		event.Symbol,
		event.Kind,
		event.Action,
		event.At.UTC().Format(time.RFC3339Nano),
		executionJSON,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"store: write lifecycle event failed",
			err,
		))
	}

	return nil
}

func (store *SQLite) writeCaptureOperation(
	executor sqliteExecutor,
	operation writerOperation,
) error {
	if !operation.identity.Valid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: capture identity required",
			nil,
		))
	}

	storedPayload, encoding := store.encodePayload(operation.payload)

	if _, err := executor.Exec(
		`INSERT INTO events
		 (kind, endpoint, at, data, run_id, capture_seq, stream, stream_epoch, stream_seq, encoding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		operation.captureKind,
		operation.endpoint,
		operation.at.UTC().Format(time.RFC3339Nano),
		storedPayload,
		string(operation.identity.Run),
		uint64(operation.identity.Sequence),
		string(operation.identity.Stream),
		uint64(operation.identity.StreamEpoch),
		operation.identity.StreamSequence,
		encoding,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf(
				"store: write %s capture failed [%s]",
				operation.captureKind,
				err.Error(),
			),
			err,
		))
	}

	return nil
}

func (store *SQLite) writeManifestOperation(
	executor sqliteExecutor,
	manifest hindsight.EnvelopeManifest,
) error {
	if !manifest.Envelope.Origin.Valid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store: envelope manifest requires a valid origin",
			nil,
		))
	}

	ref, err := hindsight.MarshalEnvelopeRef(manifest.Envelope)

	if err != nil {
		return err
	}

	payload, err := hindsight.MarshalManifest(manifest)

	if err != nil {
		return err
	}

	if _, err := executor.Exec(
		`INSERT INTO envelopes
		 (envelope_ref, origin_run, origin_seq, ordinal, workload, domain_kind, symbol, manifest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ref,
		string(manifest.Envelope.Origin.Run),
		uint64(manifest.Envelope.Origin.Sequence),
		manifest.Envelope.Ordinal,
		manifest.Workload,
		manifest.DomainKind,
		manifest.Symbol,
		payload,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("store: write envelope manifest failed [%s]", err.Error()),
			err,
		))
	}

	return nil
}
