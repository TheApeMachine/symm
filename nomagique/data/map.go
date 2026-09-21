package data

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MapServer applies a function to every element of a collection.

The function is a capability wired into the body port, so the graph draws it
as an ordinary connection to whatever node implements it. Cap'n Proto
interfaces are first-class, so the body is a live reference Map calls back
once per element inside a single observation. A collection is therefore
mapped whole, rather than one element per evaluation.

A path segment that parses as an integer indexes an array; every other
segment keys an object. Where a collection's elements are structures, the
element path names the value within each element to transform.
*/
type MapServer struct {
	*runtime.System
	path    string
	element string
	out     []byte
	count   int64
}

func NewMap(ctx context.Context) *MapServer {
	server := &MapServer{
		System: runtime.NewSystem(ctx, "data.map"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write applies the wired body to every element of the addressed collection.
*/
func (server *MapServer) Write(ctx context.Context, call Map_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.map.Write] failed to read path argument",
			err,
		))
	}

	if len(path) > 0 {
		server.path, server.element = splitCollectionPath(path)
	}

	if server.path == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.map.Write] path is not defined",
			nil,
		))
	}

	body := call.Args().Body()

	if !body.IsValid() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.map.Write] no function is wired to body",
			nil,
		))
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.map.Write] failed to read data argument",
			err,
		))
	}

	server.count = 0
	server.out = nil

	if len(payload) == 0 {
		return nil
	}

	var document any

	if err := sonic.Unmarshal(payload, &document); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.map.Write] payload is not a structure",
			err,
		))
	}

	collection, found := walk(document, strings.Split(server.path, "."))

	if !found {
		return nil
	}

	elements, ok := collection.([]any)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.map.Write] path "+server.path+" does not address a collection",
			nil,
		))
	}

	if err := server.apply(ctx, body, elements); err != nil {
		return err
	}

	encoded, err := sonic.Marshal(document)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.map.Write] failed to encode structure",
			err,
		))
	}

	server.out = encoded
	return nil
}

/*
Done emits the structure carrying the transformed collection.
*/
func (server *MapServer) Done(ctx context.Context, call Map_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.map.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetCount(server.count)

	if len(server.out) == 0 {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.map.Done] failed to set out",
			err,
		))
	}

	server.out = nil
	return nil
}

/*
apply calls the wired function once per element, replacing each element with
what the function returned.
*/
func (server *MapServer) apply(
	ctx context.Context, body Transform, elements []any,
) error {
	for index, element := range elements {
		value, found := server.read(element)

		if !found {
			continue
		}

		future, release := body.Apply(ctx, func(params Transform_apply_Params) error {
			params.SetValue(value)
			return nil
		})

		results, err := future.Struct()

		if err != nil {
			release()

			return errnie.Error(errnie.Err(
				errnie.IO,
				"[data.map.apply] the wired function failed",
				err,
			))
		}

		transformed := results.Out()
		release()

		server.write(elements, index, element, transformed)
		server.count++
	}

	return nil
}

/*
read takes the value an element contributes to the function.
*/
func (server *MapServer) read(element any) (float64, bool) {
	if server.element == "" {
		return numeric(element)
	}

	object, ok := element.(map[string]any)

	if !ok {
		return 0, false
	}

	return numeric(object[server.element])
}

/*
write places what the function returned back into the collection.
*/
func (server *MapServer) write(
	elements []any, index int, element any, transformed float64,
) {
	if server.element == "" {
		elements[index] = transformed
		return
	}

	object, ok := element.(map[string]any)

	if !ok {
		return
	}

	object[server.element] = transformed
}

/*
splitCollectionPath separates the path addressing the collection from the
path addressing a value within each of its elements. A trailing segment after
a "[]" marker names the value inside each element.
*/
func splitCollectionPath(path string) (collection, element string) {
	marker := strings.Index(path, "[].")

	if marker < 0 {
		return path, ""
	}

	return path[:marker], path[marker+3:]
}
