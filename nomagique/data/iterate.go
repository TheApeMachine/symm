package data

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
IterateServer queues incoming collections and emits one element per evaluation.
Every arrival is retained, including arrivals while an earlier collection drains.
Envelope mode retains the enclosing document and replaces the collection with
its current element, so graph consumers keep the record's channel metadata.
*/
type IterateServer struct {
	*runtime.System
	path         string
	indexPath    string
	envelope     bool
	pending      [][][]byte
	cursor       int
	out          []byte
	index, count int64
	last, found  bool
	ignored      uint64
}

func NewIterate(ctx context.Context) *IterateServer {
	server := &IterateServer{
		System: runtime.NewSystem(ctx, "data.iterate"),
	}

	server.Transition(runtime.READY)
	return server
}

/* Write admits every arriving collection, then advances one record. */
func (server *IterateServer) Write(ctx context.Context, call Iterate_write) error {
	path, err := call.Args().Path()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iterate: path", err))
	}
	indexPath, err := call.Args().IndexPath()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iterate: index path", err))
	}

	if indexPath != "" && (!call.Args().Envelope() || path == "") {
		return errnie.Error(errnie.Err(errnie.Validation, "iterate: index path requires envelope projection", nil))
	}

	if len(server.pending) > 0 && (path != server.path || call.Args().Envelope() != server.envelope || indexPath != server.indexPath) {
		return errnie.Error(errnie.Err(errnie.Validation, "iterate: cannot change projection while collections are pending", nil))
	}
	server.path, server.envelope = path, call.Args().Envelope()
	server.indexPath = indexPath
	arrivals, err := call.Args().Data()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "iterate: collections", err))
	}
	for index := 0; index < arrivals.Len(); index++ {
		payload, err := arrivals.At(index)
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "iterate: collection", err))
		}
		if len(payload) == 0 {
			continue
		}
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
	results.SetPending(uint64(len(server.pending)))
	results.SetIgnored(server.ignored)
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
load decodes and queues a collection without replacing an unfinished arrival.
*/
func (server *IterateServer) load(payload []byte) error {
	var document any

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	if err := decoder.Decode(&document); err != nil {
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
			server.ignored++
			return nil
		}

		collection = resolved
	}

	elements, ok := collection.([]any)

	// A frame whose data is one object (an instrument catalogue, a single
	// book) is one record, not a malformed collection.
	if object, single := collection.(map[string]any); !ok && single {
		elements, ok = []any{object}, true
	}

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.iterate.load] path "+server.path+" does not address a collection",
			nil,
		))
	}

	encodedElements := make([][]byte, 0, len(elements))

	if server.indexPath != "" {
		_, found := walk(document, strings.Split(server.indexPath, "."))

		if found || server.indexPath == server.path || strings.HasPrefix(server.path, server.indexPath+".") || strings.HasPrefix(server.indexPath, server.path+".") {
			return errnie.Error(errnie.Err(errnie.Validation, "iterate: index path would overwrite input", nil))
		}
	}

	for index, element := range elements {
		projected := element
		if server.envelope && server.path != "" {
			parent := document
			segments := strings.Split(server.path, ".")
			if len(segments) > 1 {
				parent, _ = walk(document, segments[:len(segments)-1])
			}
			name := segments[len(segments)-1]
			position, indexed := arrayIndex(name)
			if indexed {
				parent.([]any)[position] = element
			}
			if !indexed {
				parent.(map[string]any)[name] = element
			}
			projected = document
		}

		if server.indexPath != "" {
			indexed, err := insertAt(projected, strings.Split(server.indexPath, "."), json.Number(strconv.Itoa(index)))

			if err != nil {
				return err
			}
			projected = indexed
		}

		encoded, err := sonic.Marshal(projected)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.iterate.load] failed to encode element",
				err,
			))
		}

		encodedElements = append(encodedElements, encoded)
	}

	if len(encodedElements) > 0 {
		server.pending = append(server.pending, encodedElements)
	}
	return nil
}

/*
advance moves the cursor onto the next element.
*/
func (server *IterateServer) advance() error {
	server.found, server.last = false, false
	if len(server.pending) == 0 {
		return nil
	}
	collection := server.pending[0]
	server.out = collection[server.cursor]
	collection[server.cursor] = nil
	server.index, server.count = int64(server.cursor), int64(len(collection))
	server.found = true
	server.cursor++
	server.last = server.cursor == len(collection)
	if server.last {
		server.pending[0] = nil
		server.pending = server.pending[1:]
		server.cursor = 0
	}
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
