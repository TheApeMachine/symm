package data

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
IterateServer walks the array at a path one element per evaluation, so that
whatever a graph wires downstream of it is applied to every element in turn.
Composing it with the rest of the vocabulary is how a graph maps, filters or
reduces a collection without any node taking a function.

The graph is a directed acyclic graph, so an element cannot be fanned out
into repeated passes of the same nodes within one evaluation. Iterate is
therefore a cursor rather than a loop: it holds the collection it was given
and advances through it, publishing index, count and last so downstream
nodes can tell where in the collection they are, and re-reads only once the
collection is exhausted.
*/
type IterateServer struct {
	*runtime.System
	path     string
	elements [][]byte
	cursor   int
	out      []byte
	index    int64
	count    int64
	last     bool
	found    bool
}

func NewIterate(ctx context.Context) *IterateServer {
	server := &IterateServer{
		System: runtime.NewSystem(ctx, "data.iterate"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write loads a collection when the previous one is exhausted, then advances
the cursor by one element.
*/
func (server *IterateServer) Write(ctx context.Context, call Iterate_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.iterate.Write] failed to read path argument",
			err,
		))
	}

	if len(path) > 0 {
		server.path = path
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.iterate.Write] failed to read data argument",
			err,
		))
	}

	if server.cursor >= len(server.elements) && len(payload) > 0 {
		if err := server.load(payload); err != nil {
			return err
		}
	}

	return server.advance()
}

/*
Done emits the element the cursor rests on.
*/
func (server *IterateServer) Done(ctx context.Context, call Iterate_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.iterate.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetFound(server.found)
	results.SetIndex(server.index)
	results.SetCount(server.count)
	results.SetLast(server.last)

	if !server.found {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.iterate.Done] failed to set out",
			err,
		))
	}

	server.found = false
	server.out = nil

	return nil
}

/*
load decodes the collection at the configured path, replacing whatever the
cursor had left.
*/
func (server *IterateServer) load(payload []byte) error {
	var document any

	if err := sonic.Unmarshal(payload, &document); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.iterate.load] payload is not a structure",
			err,
		))
	}

	collection := document

	if server.path != "" {
		resolved, found := walk(document, strings.Split(server.path, "."))

		if !found {
			server.elements = nil
			server.cursor = 0
			return nil
		}

		collection = resolved
	}

	elements, ok := collection.([]any)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.iterate.load] path "+server.path+" does not address a collection",
			nil,
		))
	}

	server.elements = make([][]byte, 0, len(elements))

	for _, element := range elements {
		encoded, err := sonic.Marshal(element)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.iterate.load] failed to encode element",
				err,
			))
		}

		server.elements = append(server.elements, encoded)
	}

	server.cursor = 0
	return nil
}

/*
advance moves the cursor onto the next element.
*/
func (server *IterateServer) advance() error {
	server.found = false
	server.count = int64(len(server.elements))

	if server.cursor >= len(server.elements) {
		server.last = false
		return nil
	}

	server.out = server.elements[server.cursor]
	server.index = int64(server.cursor)
	server.found = true
	server.cursor++
	server.last = server.cursor >= len(server.elements)

	return nil
}

/*
walk resolves a path to whatever it addresses, rather than to a number.
*/
func walk(document any, segments []string) (any, bool) {
	current := document

	for _, segment := range segments {
		index, indexed := arrayIndex(segment)

		if indexed {
			elements, ok := current.([]any)

			if !ok || index >= len(elements) {
				return nil, false
			}

			current = elements[index]
			continue
		}

		object, ok := current.(map[string]any)

		if !ok {
			return nil, false
		}

		current, ok = object[segment]

		if !ok {
			return nil, false
		}
	}

	return current, true
}
