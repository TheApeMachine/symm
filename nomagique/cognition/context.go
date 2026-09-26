package cognition

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
)

/* ContextBuilderServer owns validation and construction of causal inventory contexts. */
type ContextBuilderServer struct {
	message *capnp.Message
	flat    Context
}

func NewContextBuilder() *ContextBuilderServer { return &ContextBuilderServer{} }

/* Write retains one typed context without any intermediate document encoding. */
func (server *ContextBuilderServer) Write(ctx context.Context, call ContextBuilder_write) error {
	server.Shutdown()
	args := call.Args()
	tokens, err := args.Tokens()
	if err != nil {
		return errnie.Error(err)
	}
	if tokens.Len() == 0 {
		return nil
	}
	symbol, err := args.Symbol()
	if err != nil {
		return errnie.Error(err)
	}
	vocabulary, err := args.Vocabulary()
	if err != nil {
		return errnie.Error(err)
	}
	history, err := args.History()
	if err != nil {
		return errnie.Error(err)
	}
	if symbol == "" || vocabulary == "" || args.Replay() && history.Len() == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "context: symbol, vocabulary and replay history are required", nil))
	}
	for _, values := range []capnp.TextList{tokens, history} {
		for index := range values.Len() {
			value, err := values.At(index)
			if err != nil {
				return errnie.Error(err)
			}
			if value == "" {
				return errnie.Error(errnie.Err(errnie.Validation, "context: empty token or history step", nil))
			}
		}
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return errnie.Error(err)
	}
	server.message = message
	server.flat, err = NewRootContext(segment)
	if err != nil {
		return errnie.Error(err)
	}
	for _, err := range []error{server.flat.SetSymbol(symbol), server.flat.SetVocabulary(vocabulary), server.flat.SetTokens(tokens)} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	server.flat.SetLive()
	if args.Replay() {
		if err := server.flat.SetHistory(history); err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* Done publishes inventory alternatives sharing exactly the same causal evidence. */
func (server *ContextBuilderServer) Done(ctx context.Context, call ContextBuilder_done) error {
	defer server.Shutdown()
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	if !server.flat.IsValid() {
		result.SetIdle()
		return nil
	}
	result.SetReady()
	if err := result.Ready().SetFlat(server.flat); err != nil {
		return errnie.Error(err)
	}
	if err := result.Ready().SetHeld(server.flat); err != nil {
		return errnie.Error(err)
	}
	held, err := result.Ready().Held()
	if err != nil {
		return errnie.Error(err)
	}
	held.SetHolding(true)
	return nil
}

/* Shutdown releases only the node's owned Cap'n Proto message. */
func (server *ContextBuilderServer) Shutdown() {
	if server.message != nil {
		server.message.Release()
	}
	server.message, server.flat = nil, Context{}
}
