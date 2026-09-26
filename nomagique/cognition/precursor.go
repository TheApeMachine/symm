package cognition

import (
	"context"
	"strconv"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/* PrecursorServer owns bounded live history and the shared training/live key encoding. */
type PrecursorServer struct {
	extent    uint64
	histories map[string]precursorHistory
	key       string
	prefix    bool
}

type precursorHistory struct {
	scope string
	steps []string
}

func NewPrecursor() *PrecursorServer {
	return &PrecursorServer{histories: make(map[string]precursorHistory)}
}

/* Write reads typed causal inputs; training labels never enter the selection key. */
func (server *PrecursorServer) Write(ctx context.Context, call Precursor_write) error {
	server.key, server.prefix = "", false
	input, err := call.Args().Context()

	if err != nil {
		return errnie.Error(err)
	}
	if !input.IsValid() {
		return nil
	}
	symbol, err := input.Symbol()
	if err != nil {
		return errnie.Error(err)
	}
	vocabulary, err := input.Vocabulary()
	if err != nil {
		return errnie.Error(err)
	}
	tokens, err := input.Tokens()
	if err != nil {
		return errnie.Error(err)
	}
	if symbol == "" || vocabulary == "" || tokens.Len() == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "precursor: symbol, vocabulary and active tokens are required", nil))
	}
	model := call.Args().Model()
	if !model.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "precursor: model capability is required", nil))
	}
	future, release := model.Measure(ctx, nil)
	defer release()
	measured, err := future.Struct()
	if err != nil {
		return errnie.Error(err)
	}
	server.extent = measured.Extent()
	var scope strings.Builder
	// Length prefixes distinguish arbitrary symbols and vocabularies without JSON escaping.
	scope.WriteString(strconv.Itoa(len(vocabulary)))
	scope.WriteByte(':')
	scope.WriteString(vocabulary)
	scope.WriteString(strconv.FormatBool(input.Holding()))
	scope.WriteString(strconv.Itoa(len(symbol)))
	scope.WriteByte(':')
	scope.WriteString(symbol)
	var steps []string
	if input.Which() == Context_Which_history {
		history, err := input.History()
		if err != nil {
			return errnie.Error(err)
		}
		steps, err = server.readSteps(history)
		if err != nil {
			return err
		}
	}
	if input.Which() == Context_Which_live {
		server.prefix = true
		current, err := server.readSteps(tokens)
		if err != nil {
			return err
		}
		history := server.histories[symbol]
		if history.scope != scope.String() {
			history = precursorHistory{scope: scope.String()}
		}
		step := strings.Join(current, ",")
		if len(history.steps) == 0 || history.steps[len(history.steps)-1] != step {
			history.steps = append(history.steps, step)
		}
		// Key bytes upper-bound the number of learned steps. This is a representation
		// bound, not a chosen market horizon. An empty model retains the current step.
		limit := max(uint64(1), server.extent)
		if uint64(len(history.steps)) > limit {
			history.steps = append([]string(nil), history.steps[len(history.steps)-int(limit):]...)
		}
		server.histories[symbol] = history
		steps = history.steps
	}
	if len(steps) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "precursor: training history is empty", nil))
	}
	var key strings.Builder
	key.WriteString(scope.String())
	key.WriteByte(':')
	for index := len(steps) - 1; index >= 0; index-- {
		key.WriteString(strconv.Itoa(len(steps[index])))
		key.WriteByte(':')
		key.WriteString(steps[index])
	}
	server.key = key.String()
	return nil
}

/* readSteps validates each typed token without parsing a serialized document. */
func (server *PrecursorServer) readSteps(input capnp.TextList) ([]string, error) {
	steps := make([]string, input.Len())
	for index := range input.Len() {
		value, err := input.At(index)
		if err != nil {
			return nil, errnie.Error(err)
		}
		if value == "" {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "precursor: empty history step", nil))
		}
		steps[index] = value
	}
	return steps, nil
}

func (server *PrecursorServer) Done(ctx context.Context, call Precursor_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	if server.key == "" {
		result.SetIdle()
		return nil
	}
	result.SetReady()
	result.Ready().SetPrefix(server.prefix)
	if err := result.Ready().SetKey(server.key); err != nil {
		return errnie.Error(err)
	}
	server.key = ""
	return nil
}
