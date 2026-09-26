package cognition

import (
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TokenSequenceServer maintains retained precursor token sequences per symbol/scope.
Edges in the predictive radix trie encode sequences of region tokens leading to an event,
ensuring distinct precursor paths never collapse onto the same learned action distribution.
*/
type TokenSequenceServer struct {
	*runtime.System
	history  map[string][]string
	sequence []string
	path     string
	depth    int64
}

func NewTokenSequence() *TokenSequenceServer {
	server := &TokenSequenceServer{
		System:  runtime.NewSystem(context.Background(), "cognition.token_sequence"),
		history: make(map[string][]string),
	}

	server.Transition(runtime.READY)
	return server
}

func (server *TokenSequenceServer) Write(ctx context.Context, call TokenSequence_write) error {
	args := call.Args()
	server.sequence = nil
	server.path = ""
	server.depth = 0

	scope, err := args.Scope()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.token_sequence: failed to read scope",
			err,
		))
	}

	token, err := args.Token()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.token_sequence: failed to read token",
			err,
		))
	}

	tokensList, err := args.Tokens()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.token_sequence: failed to read tokens",
			err,
		))
	}

	// A reset starts a new window: no scope carries history into it.
	if args.Reset() {
		server.history = make(map[string][]string)
	}

	var incomingTokens []string

	if tokensList.IsValid() && tokensList.Len() > 0 {
		for index := range tokensList.Len() {
			item, itemErr := tokensList.At(index)

			if itemErr == nil && strings.TrimSpace(item) != "" {
				incomingTokens = append(incomingTokens, strings.TrimSpace(item))
			}
		}
	}

	trimmed := strings.TrimSpace(token)

	if trimmed != "" {
		incomingTokens = append(incomingTokens, trimmed)
	}

	if len(incomingTokens) > 0 {
		tokenStr := strings.Join(incomingTokens, ",")
		existing := server.history[scope]

		if len(existing) == 0 || existing[len(existing)-1] != tokenStr {
			updated := make([]string, len(existing)+1)
			copy(updated, existing)
			updated[len(existing)] = tokenStr
			server.history[scope] = updated
		}
	}

	updated := server.history[scope]

	if len(updated) == 0 {
		return nil
	}

	server.sequence = updated
	server.path = strings.Join(updated, "/")
	server.depth = int64(len(updated))

	return nil
}

func (server *TokenSequenceServer) Done(ctx context.Context, call TokenSequence_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.token_sequence: failed to allocate results",
			err,
		))
	}

	results.SetIdle()

	if len(server.sequence) == 0 {
		return nil
	}

	results.SetStep()
	step := results.Step()
	step.SetDepth(server.depth)

	if err := step.SetPath(server.path); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.token_sequence: failed to set path",
			err,
		))
	}

	seqList, err := step.NewSequence(int32(len(server.sequence)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.token_sequence: failed to allocate sequence list",
			err,
		))
	}

	for index, item := range server.sequence {
		if err := seqList.Set(index, item); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"cognition.token_sequence: failed to set sequence element",
				err,
			))
		}
	}

	server.sequence = nil
	server.path = ""
	server.depth = 0
	return nil
}
