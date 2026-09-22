package cognition

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

/*
MemoryServer is the learned memory as a node in the graph.

One of these is wired to every node that reads or writes what has been
learned, which is what makes them one memory rather than several. A writer
and a reader holding separate structures is the failure this exists to
prevent: nothing errors, nothing is slow, and nothing is learned.
*/
type MemoryServer struct {
	trie *Trie
}

func NewMemory() *MemoryServer {
	return &MemoryServer{trie: NewTrie()}
}

/*
Reinforce records that a sequence was observed.
*/
func (server *MemoryServer) Reinforce(ctx context.Context, call Memory_reinforce) error {
	key, err := call.Args().Key()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.memory.Reinforce] failed to read the key",
			err,
		))
	}

	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.memory.Reinforce] failed to allocate results",
			err,
		))
	}

	if len(key) == 0 {
		return nil
	}

	// The key points into the message it arrived in, and that buffer is
	// reused by the next call. Retaining it would leave earlier keys reading
	// as whatever was written over them.
	weight, lastSeen := server.trie.Reinforce(bytes.Clone(key))
	results.SetWeight(weight)
	results.SetLastSeen(lastSeen)

	return nil
}

/*
Basin reports everything observed under one prefix.
*/
func (server *MemoryServer) Basin(ctx context.Context, call Memory_basin) error {
	prefix, err := call.Args().Prefix()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.memory.Basin] failed to read the prefix",
			err,
		))
	}

	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.memory.Basin] failed to allocate results",
			err,
		))
	}

	if len(prefix) == 0 {
		return nil
	}

	prefix = bytes.Clone(prefix)
	classes := make([][]byte, 0)
	weights := make([]uint64, 0)

	iterator := server.trie.Root().Load().Root().Iterator()
	iterator.SeekPrefix(prefix)

	for key, record, ok := iterator.Next(); ok; key, record, ok = iterator.Next() {
		if !bytes.HasPrefix(key, prefix) {
			break
		}

		class := key[len(prefix):]

		if len(class) == 0 {
			continue
		}

		classes = append(classes, bytes.Clone(class))
		weights = append(weights, WeightOf(record))
	}

	if len(classes) == 0 {
		return nil
	}

	held, err := results.NewClasses(int32(len(classes)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.memory.Basin] failed to allocate the classes",
			err,
		))
	}

	carried, err := results.NewWeights(int32(len(weights)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.memory.Basin] failed to allocate the weights",
			err,
		))
	}

	for index := range classes {
		if err := held.Set(index, classes[index]); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[cognition.memory.Basin] failed to set a class",
				err,
			))
		}

		carried.Set(index, weights[index])
	}

	return nil
}

/*
Steps is how much has been recorded, which is what tells an empty memory from
one that has been observing and found nothing.
*/
func (server *MemoryServer) Steps(ctx context.Context, call Memory_steps) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.memory.Steps] failed to allocate results",
			err,
		))
	}

	results.SetOut(server.trie.Steps().Load())
	return nil
}
