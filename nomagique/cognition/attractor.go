package cognition

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AttractorServer answers what a context most strongly attracts to: of every
class observed in that basin, the one carrying the most reinforcement.

It reports the share of the basin's reinforcement the winner holds, so a
class that has always followed a context is distinguished from one that
merely followed it most often. A context never observed attracts to nothing,
and reports so rather than naming an arbitrary class.
*/
type AttractorServer struct {
	*runtime.System
	trie  *Trie
	class []byte
	prob  float64
	count int64
}

func NewAttractor(ctx context.Context) *AttractorServer {
	server := &AttractorServer{
		System: runtime.NewSystem(ctx, "cognition.attractor"),
		trie:   NewTrie(),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Observe binds this node to the trie a writer reinforces, so what it walks is
what was learned.
*/
func (server *AttractorServer) Observe(trie *Trie) { server.trie = trie }

/*
Write walks the basin and settles on the class carrying the most weight.
*/
func (server *AttractorServer) Write(ctx context.Context, call Attractor_write) error {
	contextBytes, err := call.Args().ContextBytes()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[cognition.attractor.Write] failed to read context argument",
			err,
		))
	}

	server.class = nil
	server.prob = 0
	server.count = 0

	if len(contextBytes) == 0 {
		return nil
	}

	prefix := BasinPrefixOf(basinOf(contextBytes))
	iterator := server.trie.Root().Load().Root().Iterator()
	iterator.SeekPrefix(prefix)

	var winner []byte
	var winning, total uint64

	for key, record, ok := iterator.Next(); ok; key, record, ok = iterator.Next() {
		if !bytes.HasPrefix(key, prefix) {
			break
		}

		class := key[len(prefix):]

		if len(class) == 0 {
			continue
		}

		weight := WeightOf(record)
		total += weight
		server.count++

		if weight > winning {
			winning = weight
			winner = bytes.Clone(class)
		}
	}

	if total == 0 {
		return nil
	}

	server.class = winner
	server.prob = float64(winning) / float64(total)

	return nil
}

/*
Done reports the class the context attracts to and how much of the basin's
reinforcement it holds.
*/
func (server *AttractorServer) Done(ctx context.Context, call Attractor_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Done] failed to allocate results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetCount(server.count)

	if len(server.class) == 0 {
		return nil
	}

	if err := results.SetClass(server.class); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[cognition.attractor.Done] failed to set class",
			err,
		))
	}

	server.class = nil
	return nil
}

/*
basinOf reads the context out of a basin key, so a key emitted by a writer can
be handed straight back as the context to look up.
*/
func basinOf(contextBytes []byte) []byte {
	if !bytes.HasPrefix(contextBytes, []byte("b/")) {
		return contextBytes
	}

	trimmed := contextBytes[2:]
	cut := bytes.LastIndexByte(trimmed, '/')

	if cut < 0 {
		return trimmed
	}

	return trimmed[:cut]
}
