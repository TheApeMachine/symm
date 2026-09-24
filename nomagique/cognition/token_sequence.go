package cognition

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
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
	out      []byte
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
	server.out = nil

	scope, err := args.Scope()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.token_sequence: failed to read scope",
			err,
		))
	}

	token, _ := args.Token()
	tokensList, _ := args.Tokens()

	if args.Reset() {
		if scope != "" {
			delete(server.history, scope)
		} else {
			server.history = make(map[string][]string)
		}
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

	if len(incomingTokens) == 0 {
		return nil
	}

	stepToken := strings.Join(incomingTokens, ",")
	existing := server.history[scope]
	updated := make([]string, len(existing)+1)
	copy(updated, existing)
	updated[len(existing)] = stepToken

	server.history[scope] = updated
	server.sequence = updated
	server.path = strings.Join(updated, "/")
	server.depth = int64(len(updated))

	encoded, err := sonic.Marshal(server.path)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.token_sequence: failed to encode output",
			err,
		))
	}

	server.out = encoded
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

	results.SetDepth(server.depth)

	if err := results.SetPath(server.path); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.token_sequence: failed to set path",
			err,
		))
	}

	seqList, err := results.NewSequence(int32(len(server.sequence)))
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

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"cognition.token_sequence: failed to set out payload",
				err,
			))
		}
	}

	server.sequence = nil
	server.path = ""
	server.depth = 0
	server.out = nil
	return nil
}
