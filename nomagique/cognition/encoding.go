package cognition

import (
	"bytes"
	"encoding/gob"
	"io"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

// Encode serializes one immutable trie snapshot, its configuration and clock.
// There is no checkpoint model: the values are the same packed weights read
// by Evaluate. Storage I/O belongs to the caller.
func (engine *Engine) Encode() ([]byte, error) {
	root := engine.root.Load()
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	for _, value := range []any{"cognition/packed-weight/1", engine.cfg, engine.stepCounter.Load(), root.Len()} {
		if err := encoder.Encode(value); err != nil {
			return nil, errnie.Error(err)
		}
	}
	iterator := root.Root().Iterator()
	for key, value, found := iterator.Next(); found; key, value, found = iterator.Next() {
		if err := encoder.Encode(key); err != nil {
			return nil, errnie.Error(err)
		}

		if err := encoder.Encode(value); err != nil {
			return nil, errnie.Error(err)
		}
	}
	return buffer.Bytes(), nil
}

// Decode restores into a fresh engine before it is shared. Invalid or retired
// formats fail explicitly; neither partial state nor a replacement empty model
// is published after a failed read.
func (engine *Engine) Decode(encoded []byte) error {
	if engine.root.Load().Len() != 0 || engine.stepCounter.Load() != 0 {
		return errnie.Error(errnie.Err(errnie.Conflict, "cognition: restore requires a fresh engine", nil))
	}
	decoder := gob.NewDecoder(bytes.NewReader(encoded))
	var format string
	var config Config
	var step uint64
	var count int
	for _, destination := range []any{&format, &config, &step, &count} {
		if err := decoder.Decode(destination); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model header", err))
		}
	}

	if format != "cognition/packed-weight/1" || count < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: unsupported packed model format", nil))
	}

	if config.DirichletAlpha <= 0 || config.MaxBackoffOrder <= 0 || config.SurprisalBreakBits <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid model configuration", nil))
	}
	transaction := iradix.New[[]byte]().Txn()
	for range count {
		var key, value []byte
		for _, destination := range []any{&key, &value} {
			if err := decoder.Decode(destination); err != nil {
				return errnie.Error(errnie.Err(errnie.Validation, "cognition: incomplete packed model", err))
			}
		}

		if err := engine.validate(key, value, step); err != nil {
			return err
		}

		if _, replaced := transaction.Insert(key, value); replaced {
			return errnie.Error(errnie.Err(errnie.Validation, "cognition: duplicate packed model key", nil))
		}
	}
	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: trailing packed model data", err))
	}
	engine.cfg, engine.decayFactor = config, config.DecayFactor()
	engine.stepCounter.Store(step)
	engine.root.Store(transaction.Commit())
	return nil
}

func (engine *Engine) validate(key, value []byte, step uint64) error {
	_, _, basin := parseBasinKey(key)
	sensory := bytes.HasPrefix(key, []byte("s/")) && len(key) > len("s/")

	if (!basin && !sensory) || len(value) != WeightSize {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model record", nil))
	}
	weight := DecodeWeight(value)

	if weight.Count == 0 || weight.WriteStep > step || weight.Probability < 0 || weight.Probability > 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model weight", nil))
	}
	return nil
}
