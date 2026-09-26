package execution

import (
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* pollDurability never waits in the market path. A failed save blocks new risk. */
func (server *AccountServer) pollDurability() error {
	if server.releaseSave == nil {
		return nil
	}
	select {
	case <-server.saving.Done():
		_, err := server.saving.Struct()
		server.releaseSave()
		server.releaseSave = nil
		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "execution account: checkpoint failed; new risk blocked", err))
		}
		server.durableRevision = server.savingRevision
		server.persistenceError = ""
		return nil
	default:
		return nil
	}
}

/*
	persist dispatches one immutable full-state save. Later transitions remain

on the canonical owner and are saved after the in-flight revision completes.
*/
func (server *AccountServer) persist(ctx context.Context) error {
	if !server.live || server.cash == nil || server.releaseSave != nil || server.durableRevision >= server.revision {
		return nil
	}
	if !server.checkpoint.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: live checkpoint capability missing", nil))
	}
	encoded, err := server.snapshotBytes()
	if err != nil {
		return err
	}
	server.savingRevision = server.revision
	server.saving, server.releaseSave = server.checkpoint.Save(context.WithoutCancel(ctx), func(params runtime.Checkpoint_save_Params) error {
		if err := params.SetKey(server.checkpointKey); err != nil {
			return err
		}
		return params.SetData(encoded)
	})
	return nil
}

/* Flush is the explicit shutdown fence, where waiting for disk is permitted. */
func (server *AccountServer) Flush(ctx context.Context, call runtime.Durable_flush) error {
	if !server.live {
		return nil
	}
	for {
		if server.releaseSave != nil {
			_, err := server.saving.Struct()
			if err != nil {
				return errnie.Error(err)
			}
			if err := server.pollDurability(); err != nil {
				return err
			}
		}
		if server.durableRevision >= server.revision {
			return nil
		}
		if err := server.persist(ctx); err != nil {
			return err
		}
	}
}

/* Shutdown releases only capability references; graph shutdown owns Flush. */
func (server *AccountServer) Shutdown() {
	if server.releaseSave != nil {
		server.releaseSave()
	}
	if server.checkpoint.IsValid() {
		server.checkpoint.Release()
	}
}
