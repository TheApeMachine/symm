package parallel

import (
	"bytes"
	"context"
	"encoding"
	"encoding/gob"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/store"
)

const (
	statePrefix  = "parallel/state/"
	ledgerKey    = "parallel/ledger.bin"
	saveEverySeq = 128
)

type symbolPersist struct {
	Features     []resonance.Feature
	Steps        int64
	Moments      [][]byte
	Standardized []float64
	Control      []byte
	Treatment    []byte
}

type ledgerPersist struct {
	Open    map[BookID]map[string]*Lot
	Returns map[BookID][]float64
}

func (runner *Runner) restore(client *store.Client) {
	if runner == nil || client == nil {
		return
	}

	for key := range client.ListKeys(statePrefix) {
		symbol := strings.TrimPrefix(key, statePrefix)
		symbol = strings.TrimSuffix(symbol, ".bin")
		if symbol == "" {
			continue
		}

		value := client.Download(context.Background(), key)
		if value == nil {
			continue
		}

		blob, err := io.ReadAll(value)
		if err != nil {
			runner.Error(err)
			continue
		}

		if err := runner.coders.LoadSymbol(symbol, blob); err != nil {
			runner.Error(err)
		}
	}

	value := client.Download(context.Background(), ledgerKey)
	if value == nil {
		return
	}

	blob, err := io.ReadAll(value)
	if err != nil {
		runner.Error(err)
		return
	}

	if err := runner.ledger.Unmarshal(blob); err != nil {
		runner.Error(err)
	}
}

func (runner *Runner) persistSymbol(symbol string) {
	if runner == nil || runner.client == nil || symbol == "" {
		return
	}

	blob, _, err := runner.coders.SaveSymbol(symbol)
	if err != nil {
		runner.Error(err)
		return
	}

	key := statePrefix + symbol + ".bin"
	if _, err := runner.client.WriteAtPrefix(key, blob); err != nil {
		runner.Error(err)
	}
}

func (runner *Runner) persistLedger() {
	if runner == nil || runner.client == nil || runner.ledger == nil {
		return
	}

	blob, err := runner.ledger.Marshal()
	if err != nil {
		runner.Error(err)
		return
	}

	if _, err := runner.client.WriteAtPrefix(ledgerKey, blob); err != nil {
		runner.Error(err)
	}
}

func (runner *Runner) maybePersist(symbol string, seq int64) {
	if seq%saveEverySeq != 0 {
		return
	}

	runner.persistSymbol(symbol)
}

func (bank *CoderBank) SaveSymbol(symbol string) ([]byte, int, error) {
	if bank == nil {
		return nil, 0, fmt.Errorf("parallel: nil coder bank")
	}

	bank.mu.Lock()
	pair := bank.pairs[symbol]
	bank.mu.Unlock()

	if pair == nil {
		return nil, 0, errnie.Err(errnie.NotFound, "parallel: no coder for "+symbol+" to save", nil)
	}

	control := pair.control
	treatment := pair.treatment

	dropped, err := dropPending(control)
	if err != nil {
		return nil, 0, err
	}

	droppedTreat, err := dropPending(treatment)
	if err != nil {
		return nil, 0, err
	}

	dropped += droppedTreat

	state := symbolPersist{
		Features:     resonance.Features,
		Standardized: slices.Clone(pair.scorer.standardized),
		Moments:      make([][]byte, len(pair.scorer.moments)),
		Steps:        pair.step,
	}

	for index, moments := range pair.scorer.moments {
		marshaler, ok := moments.(encoding.BinaryMarshaler)
		if !ok {
			return nil, 0, fmt.Errorf("parallel: %s moments not marshalable", symbol)
		}

		var err error
		if state.Moments[index], err = marshaler.MarshalBinary(); err != nil {
			return nil, 0, errnie.Err(errnie.Internal, fmt.Sprintf("parallel: %s moments[%d]", symbol, index), err)
		}
	}

	controlBlob, err := control.MarshalBinary()
	if err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "parallel: "+symbol+" control coder", err)
	}

	state.Control = controlBlob

	treatmentBlob, err := treatment.MarshalBinary()
	if err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "parallel: "+symbol+" treatment coder", err)
	}

	state.Treatment = treatmentBlob

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(state); err != nil {
		return nil, 0, errnie.Err(errnie.Internal, "parallel: encoding "+symbol+"'s state", err)
	}

	return buf.Bytes(), dropped, nil
}

func (bank *CoderBank) LoadSymbol(symbol string, blob []byte) error {
	if bank == nil {
		return fmt.Errorf("parallel: nil coder bank")
	}

	bank.mu.Lock()
	if bank.pairs[symbol] != nil {
		bank.mu.Unlock()
		return errnie.Err(errnie.Conflict, "parallel: "+symbol+" already has coders", nil)
	}
	bank.mu.Unlock()

	var state symbolPersist
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&state); err != nil {
		return errnie.Err(errnie.UnprocessableContent, "parallel: "+symbol+"'s state does not decode", err)
	}

	if !slices.Equal(state.Features, resonance.Features) ||
		len(state.Moments) != len(resonance.Features) ||
		len(state.Standardized) != len(resonance.Features) {
		return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"parallel: %s's state features %v, expected %v", symbol, state.Features, resonance.Features,
		), nil)
	}

	scorer := newResonanceFeatureScorer()
	for index, moments := range scorer.moments {
		unmarshaler, ok := moments.(encoding.BinaryUnmarshaler)
		if !ok {
			return fmt.Errorf("parallel: moments not unmarshalable")
		}

		if err := unmarshaler.UnmarshalBinary(state.Moments[index]); err != nil {
			return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf("parallel: %s moments[%d]", symbol, index), err)
		}
	}

	copy(scorer.standardized, state.Standardized)

	pair := newCoderPair(bank.pace, len(resonance.Features))
	pair.scorer = scorer
	pair.step = state.Steps

	if err := pair.control.UnmarshalBinary(state.Control); err != nil {
		return errnie.Err(errnie.UnprocessableContent, "parallel: "+symbol+" control coder", err)
	}

	if err := pair.treatment.UnmarshalBinary(state.Treatment); err != nil {
		return errnie.Err(errnie.UnprocessableContent, "parallel: "+symbol+" treatment coder", err)
	}

	bank.mu.Lock()
	bank.pairs[symbol] = pair
	bank.mu.Unlock()

	return nil
}

func dropPending(coder *learning.PredictiveCoder) (int, error) {
	if coder == nil {
		return 0, nil
	}

	return coder.DropPending()
}

func (ledger *Ledger) Marshal() ([]byte, error) {
	if ledger == nil {
		return nil, fmt.Errorf("parallel: nil ledger")
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	state := ledgerPersist{
		Open:    ledger.open,
		Returns: ledger.returns,
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(state); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ledger *Ledger) Unmarshal(blob []byte) error {
	if ledger == nil {
		return fmt.Errorf("parallel: nil ledger")
	}

	var state ledgerPersist
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&state); err != nil {
		return err
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if state.Open != nil {
		ledger.open = state.Open
	}

	if state.Returns != nil {
		ledger.returns = state.Returns
	}

	return nil
}
