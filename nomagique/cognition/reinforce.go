package cognition

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ReinforceServer records that a class was observed in a context, strengthening
that sequence in the shared trie. What it emits is the basin key it recorded,
so the node reading the trie walks exactly what was written.

Reinforcement is the whole of the learning here: a sequence observed often
carries more weight than one observed once, and nothing is weighted by
anything other than having happened.
*/
type ReinforceServer struct {
	*runtime.System
	out    []byte
	weight uint64
}

func NewReinforce(ctx context.Context) *ReinforceServer {
	server := &ReinforceServer{
		System: runtime.NewSystem(ctx, "cognition.reinforce"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write reinforces the observed sequence.
*/
func (server *ReinforceServer) Write(ctx context.Context, call Reinforce_write) error {
	contextBytes, err := call.Args().ContextBytes()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.reinforce.Write] failed to read context argument",
			err,
		))
	}

	classBytes, err := call.Args().ClassBytes()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.reinforce.Write] failed to read class argument",
			err,
		))
	}

	server.out = nil
	server.weight = 0

	if len(classBytes) == 0 {
		return nil
	}

	memory := call.Args().Memory()

	// Recording into a structure of this node's own would learn perfectly
	// and teach nothing: the node that reads it would be walking another.
	if !memory.IsValid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[cognition.reinforce.Write] no learned memory is wired to this node",
			nil,
		))
	}

	key := BasinKeyOf(contextBytes, classBytes)

	future, release := memory.Reinforce(ctx, func(params Memory_reinforce_Params) error {
		return params.SetKey(key)
	})
	defer release()

	recorded, err := future.Struct()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.reinforce.Write] failed to record the observed sequence",
			err,
		))
	}

	server.out = key
	server.weight = recorded.Weight()

	return nil
}

/*
Done emits the basin key that was reinforced.
*/
func (server *ReinforceServer) Done(ctx context.Context, call Reinforce_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.reinforce.Done] failed to allocate results",
			err,
		))
	}

	if len(server.out) == 0 {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.reinforce.Done] failed to set out",
			err,
		))
	}

	server.out = nil
	return nil
}

/*
BasinKeyOf builds the key one observation is recorded under: the basin the
observation was made in, then the class observed there. Keying this way means
a reader seeking a context walks exactly the classes seen in it.
*/
func BasinKeyOf(contextBytes, classBytes []byte) []byte {
	key := make([]byte, 0, 2+len(contextBytes)+1+len(classBytes))

	key = append(key, 'b', '/')
	key = append(key, contextBytes...)
	key = append(key, '/')
	key = append(key, classBytes...)

	return key
}

/*
BasinPrefixOf builds the prefix every class observed in one basin shares.
*/
func BasinPrefixOf(contextBytes []byte) []byte {
	prefix := make([]byte, 0, 2+len(contextBytes)+1)

	prefix = append(prefix, 'b', '/')
	prefix = append(prefix, contextBytes...)
	prefix = append(prefix, '/')

	return prefix
}
