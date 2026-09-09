package strategy

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

// modelKey names the cognition engine's encoded state in the object store.
const modelKey = "models/cognition.gob"

/*
Blobs is the whole-object storage the checkpoint needs.

It is an interface rather than the concrete store so a test can supply memory
instead of a live bucket: the model is the only thing left in object storage,
and standing up S3 to exercise save-and-restore would test the venue rather
than the learner.
*/
type Blobs interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, data []byte) error
}

// Save persists identities and cognition in one coherent checkpoint. Storage
// runs after releasing the processing locks.
func (learner *Learner) Save(ctx context.Context) error {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	learner.mutex.Lock()
	learner.Grid.Mutex.Lock()
	err := encoder.Encode(learner.Grid.Columns)

	if err == nil {
		var encoded []byte
		encoded, err = learner.Agent.Model.Encode()

		if err == nil {
			err = encoder.Encode(encoded)
		}
	}
	learner.Grid.Mutex.Unlock()
	learner.mutex.Unlock()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "learning: encode cognition checkpoint", err))
	}
	return errnie.Error(learner.archive.Write(ctx, modelKey, buffer.Bytes()))
}

// Restore applies a cognition checkpoint only before processing starts. The
// distinct storage key prevents interpreting the retired model's bytes as cognition.
func (learner *Learner) Restore(ctx context.Context) (bool, error) {
	data, found, err := learner.archive.Read(ctx, modelKey)

	if err != nil || !found {
		return found, errnie.Error(err)
	}
	decoder := gob.NewDecoder(bytes.NewReader(data))
	var columns [][2]string

	if err := decoder.Decode(&columns); err != nil {
		return false, errnie.Error(err)
	}
	learned := cognition.NewEngine(cognition.DefaultConfig())
	var encoded []byte

	if err := decoder.Decode(&encoded); err != nil {
		return false, errnie.Error(err)
	}

	if err := learned.Decode(encoded); err != nil {
		return false, errnie.Error(err)
	}
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	learner.Grid.Mutex.Lock()
	defer learner.Grid.Mutex.Unlock()

	if learner.Grid.Version != 0 || learner.Agent.Decisions != 0 {
		return false, errnie.Error(errnie.Err(errnie.Conflict, "learning: restore requires fresh owners", nil))
	}
	for index, identity := range columns {
		if learner.Grid.Column(identity[0], identity[1]) != index {
			return false, errnie.Error(errnie.Err(errnie.Validation, "learning: duplicate checkpoint quantity", nil))
		}
	}
	learner.Agent.Model = learned
	return true, nil
}

// Learn admits signed historical experience under the original named
// quantities. This preserves the existing consolidation path; agent renewal
// and cross-agent knowledge policies are not changed here.
func (learner *Learner) Learn(label string, columns [][2]string, context []uint64, action Action, value, authority float64) error {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()
	learner.Grid.Mutex.Lock()
	defer learner.Grid.Mutex.Unlock()
	mapped := make([]uint64, len(context))
	for index, token := range context {
		if token&(uint64(1)<<63) != 0 {
			mapped[index] = token
			continue
		}
		quantity := grid.ConditionQuantity(token)

		if quantity == 0 || quantity > uint64(len(columns)) {
			return errnie.Error(errnie.Err(errnie.Validation, "learning: rehearsal quantity has no identity", nil))
		}
		identity := columns[quantity-1]
		column := learner.Grid.Column(identity[0], identity[1])
		mapped[index] = grid.RemapCondition(token, uint64(column+1))
	}
	if authority < 0 || authority > 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "learning: feedback authority must be in [0, 1]", nil))
	}

	if authority > 0 {
		learner.Agent.Model.Observe(agent.ContextKey(label, mapped), []byte(fmt.Sprint(action)), value*authority)
	}
	return nil
}
