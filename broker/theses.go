package broker

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const (
	thesisSchemaKey   = "thesis/schema"
	thesisSchemaValue = "symm/thesis/v2"
	activeThesisKey   = "thesis/active/"
)

/*
Theses owns the one durable DMT namespace used to recover active position
Theses after a process restart. It stores domain records only; market transport
and cognitive inference remain outside this responsibility.
*/
type Theses struct {
	tree  *dmt.Tree
	uiHub chan<- []byte
}

/*
Active restores every live case so startup reconciles durable state and wallet
holdings as one set instead of silently ignoring submitted or closed records.
*/
func (theses *Theses) Active() (map[string]*types.Thesis, error) {
	payloads := make(map[string][]byte)
	timestamps := make(map[string]int64)
	var err error

	theses.tree.WalkPrefix([]byte(activeThesisKey), func(key, payload []byte) bool {
		var symbol string
		var timestamp int64
		symbol, timestamp, err = theses.decode(key)

		if err != nil {
			return false
		}

		if timestamp > timestamps[symbol] {
			timestamps[symbol] = timestamp
			payloads[symbol] = payload
		}

		return true
	})

	if err != nil {
		return nil, err
	}

	active := make(map[string]*types.Thesis, len(payloads))

	for symbol, payload := range payloads {
		var thesis *types.Thesis
		thesis, err = types.RestoreThesis(payload, theses.uiHub)

		if err != nil {
			return nil, errnie.Err(
				errnie.Validation, "failed to decode active Thesis for "+symbol, err,
			)
		}

		if thesis.LifecycleState(symbol) == types.LifecycleObserving {
			return nil, errnie.Err(
				errnie.Validation, "active Thesis has no lifecycle for "+symbol, nil,
			)
		}

		active[symbol] = thesis
	}

	return active, nil
}

/*
NewTheses binds active position recovery to one shared DMT tree and verifies
that the tree can accept durable writes before any order may be submitted.
*/
func NewTheses(
	tree *dmt.Tree,
	uiHub chan<- []byte,
) (*Theses, error) {
	if tree == nil {
		return nil, errnie.Err(errnie.Validation, "Thesis tree is required", nil)
	}

	theses := &Theses{tree: tree, uiHub: uiHub}
	existing, found := tree.Get([]byte(thesisSchemaKey))

	if found {
		if string(existing) != thesisSchemaValue {
			return nil, errnie.Err(
				errnie.Validation,
				"unsupported Thesis store schema "+string(existing),
				nil,
			)
		}

		return theses, nil
	}

	_, _, err := tree.Insert([]byte(thesisSchemaKey), []byte(thesisSchemaValue))

	if err != nil {
		return nil, errnie.Err(errnie.IO, "Thesis tree is not writable", err)
	}

	return theses, nil
}

/*
Save appends one timestamped active snapshot for a canonical symbol. Immutable
keys keep delayed older writes behind newer snapshots during recovery.
*/
func (theses *Theses) Save(symbol string, thesis *types.Thesis) error {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" || thesis == nil {
		return errnie.Err(errnie.Validation, "active Thesis and symbol are required", nil)
	}

	payload, timestamp, err := thesis.MarshalCheckpoint()

	if err != nil {
		return errnie.Err(errnie.Validation, "failed to encode active Thesis for "+symbol, err)
	}

	_, _, err = theses.tree.Insert(theses.checkpoint(symbol, timestamp), payload)

	if err != nil {
		return errnie.Err(errnie.IO, "failed to persist active Thesis for "+symbol, err)
	}

	keys := make([][]byte, 0)
	theses.tree.WalkPrefix(theses.active(symbol), func(key, _ []byte) bool {
		var observed int64
		_, observed, err = theses.decode(key)

		if err != nil {
			return false
		}

		if observed < timestamp {
			keys = append(keys, append([]byte(nil), key...))
		}

		return true
	})

	if err != nil {
		return err
	}

	for _, key := range keys {
		if _, _, err = theses.tree.Delete(key); err != nil {
			return errnie.Err(errnie.IO, "failed to prune active Thesis for "+symbol, err)
		}
	}

	return nil
}

/*
Load restores the active Thesis for one canonical symbol. The found result
distinguishes a legitimate pre-persistence holding from a corrupt record, so
Desk can expose the former as an orphan without hiding the latter.
*/
func (theses *Theses) Load(symbol string) (*types.Thesis, bool, error) {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return nil, false, errnie.Err(errnie.Validation, "Thesis symbol is required", nil)
	}

	var payload []byte
	latest := int64(0)
	var err error
	theses.tree.WalkPrefix(theses.active(symbol), func(key, value []byte) bool {
		var timestamp int64
		_, timestamp, err = theses.decode(key)

		if err != nil {
			return false
		}

		if timestamp > latest {
			latest = timestamp
			payload = value
		}

		return true
	})

	if err != nil {
		return nil, true, err
	}

	if payload == nil {
		return nil, false, nil
	}

	thesis, err := types.RestoreThesis(payload, theses.uiHub)

	if err != nil {
		return nil, true, errnie.Err(
			errnie.Validation, "failed to decode active Thesis for "+symbol, err,
		)
	}

	if thesis.LifecycleState(symbol) == types.LifecycleObserving {
		return nil, true, errnie.Err(
			errnie.Validation, "active Thesis has no lifecycle for "+symbol, nil,
		)
	}

	return thesis, true, nil
}

/*
Complete archives one evaluated case under its first entry execution identity
before removing the active recovery key. A crash between those writes leaves a
duplicate record rather than losing the completed lifecycle.
*/
func (theses *Theses) Complete(symbol string, thesis *types.Thesis) error {
	if thesis == nil || thesis.LifecycleState(symbol) != types.LifecycleEvaluated {
		return errnie.Err(errnie.Forbidden, "only an evaluated Thesis can complete "+symbol, nil)
	}

	identity := theses.identity(symbol, thesis)

	if identity == "" {
		return errnie.Err(errnie.Validation, "entry execution identity required for "+symbol, nil)
	}

	completed := []byte("thesis/completed/" + url.PathEscape(symbol) + "/" + identity)
	payload, err := thesis.MarshalBinary()

	if err != nil {
		return errnie.Err(errnie.Validation, "failed to encode completed Thesis for "+symbol, err)
	}

	_, _, err = theses.tree.Insert(completed, payload)

	if err != nil {
		return errnie.Err(errnie.IO, "failed to archive completed Thesis for "+symbol, err)
	}

	keys := make([][]byte, 0)
	theses.tree.WalkPrefix(theses.active(symbol), func(key, _ []byte) bool {
		keys = append(keys, append([]byte(nil), key...))

		return true
	})

	for _, key := range keys {
		if _, _, err = theses.tree.Delete(key); err != nil {
			return errnie.Err(errnie.IO, "failed to remove active Thesis for "+symbol, err)
		}
	}

	return nil
}

/*
active returns the timestamped recovery prefix reserved for one symbol.
*/
func (theses *Theses) active(symbol string) []byte {
	return []byte(activeThesisKey + url.PathEscape(symbol) + "/")
}

/*
checkpoint returns one lexicographically ordered immutable snapshot key.
*/
func (theses *Theses) checkpoint(symbol string, timestamp int64) []byte {
	return []byte(fmt.Sprintf("%s%020d", theses.active(symbol), timestamp))
}

/*
decode validates and separates one active snapshot key into its symbol and
nanosecond checkpoint timestamp.
*/
func (theses *Theses) decode(key []byte) (string, int64, error) {
	remainder := strings.TrimPrefix(string(key), activeThesisKey)
	encoded, timestampText, found := strings.Cut(remainder, "/")

	if !found || encoded == "" || timestampText == "" || strings.Contains(timestampText, "/") {
		return "", 0, errnie.Err(errnie.Validation, "invalid active Thesis key", nil)
	}

	symbol, err := url.PathUnescape(encoded)

	if err != nil || strings.TrimSpace(symbol) == "" {
		return "", 0, errnie.Err(errnie.Validation, "invalid active Thesis symbol", err)
	}

	timestamp, err := strconv.ParseInt(timestampText, 10, 64)

	if err != nil || timestamp <= 0 {
		return "", 0, errnie.Err(errnie.Validation, "invalid active Thesis timestamp", err)
	}

	return symbol, timestamp, nil
}

/*
identity returns the first immutable buy execution retained by the completed
case, including reconciled entries that began before the current process.
*/
func (theses *Theses) identity(symbol string, thesis *types.Thesis) string {
	for _, observation := range thesis.TradeJournal {
		if observation.Symbol != symbol || observation.ExecutionID == "" {
			continue
		}

		if observation.Side == "buy" || observation.Kind == "position_reconciliation" {
			return observation.ExecutionID
		}
	}

	return ""
}
