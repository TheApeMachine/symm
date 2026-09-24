package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
InsertServer places a number or JSON value at a dotted path. Chaining
inserts accumulates many named
values into one structure, which is how a composed graph builds up a result
without any node needing to know what the values mean.

A path segment that parses as an integer indexes an array; every other
segment keys an object. Intermediate containers are created as needed, so a
structure can be built from nothing by inserting into an empty payload.
*/
type InsertServer struct {
	*runtime.System
	path     string
	out      []byte
	inserted bool
}

func NewInsert(ctx context.Context) *InsertServer {
	server := &InsertServer{
		System: runtime.NewSystem(ctx, "data.insert"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write inserts the inbound value into the inbound structure at the configured
path, retaining the result for Done.
*/
func (server *InsertServer) Write(ctx context.Context, call Insert_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.insert.Write] failed to read path argument",
			err,
		))
	}

	server.path = path

	if server.path == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.insert.Write] path is not defined",
			nil,
		))
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.insert.Write] failed to read data argument",
			err,
		))
	}

	document, err := server.decode(payload)

	if err != nil {
		return err
	}

	var value any = call.Args().Value()
	encoding, err := call.Args().Encoding()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "insert: encoding", err))
	}

	if encoding != "" && encoding != "uint64" {
		return errnie.Error(errnie.Err(errnie.Validation, "insert: unsupported encoding", nil))
	}

	if encoding == "uint64" {
		if call.Args().HasJson() {
			return errnie.Error(errnie.Err(errnie.Validation, "insert: unsigned and JSON inputs are mutually exclusive", nil))
		}
		value = json.Number(strconv.FormatUint(call.Args().Unsigned(), 10))
	}

	if call.Args().HasJson() {
		raw, err := call.Args().Json()

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "insert: JSON value", err))
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()

		if err := decoder.Decode(&value); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "insert: invalid JSON value", err))
		}
	}
	server.inserted = true

	if call.Args().Unique() {
		previous, found := walk(document, strings.Split(server.path, "."))

		if found && !reflect.DeepEqual(previous, value) {
			fmt.Printf("DEDUPLICATE CONFLICT: path=%s\n  PREV: %v\n  CURR: %v\n", server.path, previous, value)
			return errnie.Error(errnie.Err(errnie.Validation, "insert: conflicting value at unique path "+server.path, nil))
		}

		server.inserted = !found
	}

	document, err = insertAt(document, strings.Split(server.path, "."), value)

	if err != nil {
		return err
	}

	encoded, err := sonic.Marshal(document)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.insert.Write] failed to encode structure",
			err,
		))
	}

	server.out = encoded

	return nil
}

/*
Done emits the structure carrying the inserted value.
*/
func (server *InsertServer) Done(ctx context.Context, call Insert_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.insert.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetInserted(server.inserted)

	if len(server.out) == 0 {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.insert.Done] failed to set out",
			err,
		))
	}

	server.out = nil
	server.inserted = false
	return nil
}

/*
decode reads only this evaluation. An empty payload starts a new structure.
*/
func (server *InsertServer) decode(payload []byte) (any, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var document any

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	if err := decoder.Decode(&document); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.insert.decode] inbound payload is not a structure",
			err,
		))
	}

	return document, nil
}

/*
insertAt places value into document at segments, creating the objects and
arrays the path implies.
*/
func insertAt(document any, segments []string, value any) (any, error) {
	segment := segments[0]
	index, indexed := arrayIndex(segment)

	if indexed {
		return insertIntoArray(document, segments, index, value)
	}

	object, ok := document.(map[string]any)

	if !ok {
		if document != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[data.insert] path segment "+segment+" addresses a non-object",
				nil,
			))
		}

		object = make(map[string]any)
	}

	if len(segments) == 1 {
		object[segment] = value
		return object, nil
	}

	child, err := insertAt(object[segment], segments[1:], value)

	if err != nil {
		return nil, err
	}

	object[segment] = child
	return object, nil
}

func insertIntoArray(
	document any, segments []string, index int, value any,
) (any, error) {
	elements, ok := document.([]any)

	if !ok {
		if document != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[data.insert] path segment "+segments[0]+" addresses a non-array",
				nil,
			))
		}

		elements = make([]any, 0, index+1)
	}

	for len(elements) <= index {
		elements = append(elements, nil)
	}

	if len(segments) == 1 {
		elements[index] = value
		return elements, nil
	}

	child, err := insertAt(elements[index], segments[1:], value)

	if err != nil {
		return nil, err
	}

	elements[index] = child
	return elements, nil
}

func arrayIndex(segment string) (int, bool) {
	index, err := strconv.Atoi(segment)

	if err != nil || index < 0 {
		return 0, false
	}

	return index, true
}
