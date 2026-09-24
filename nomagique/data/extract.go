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
ExtractServer projects a number, JSON value, or text from a dotted path. A path segment that parses as an integer
indexes an array; every other segment keys an object.

An absent path is reported as found=false rather than as a zero, so that a
value the structure does not carry stays distinguishable from a value it
carries as zero.
*/
type ExtractServer struct {
	*runtime.System
	path     string
	out      float64
	found    bool
	raw      []byte
	text     string
	encoding string
	unsigned uint64
}

func NewExtract(ctx context.Context) *ExtractServer {
	server := &ExtractServer{
		System: runtime.NewSystem(ctx, "data.extract"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write reads the value at the configured path out of the inbound structure.
*/
func (server *ExtractServer) Write(ctx context.Context, call Extract_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.extract.Write] failed to read path argument",
			err,
		))
	}

	server.path = path

	if server.path == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.extract.Write] path is not defined",
			nil,
		))
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.extract.Write] failed to read data argument",
			err,
		))
	}

	if len(payload) == 0 {
		server.found = false
		return nil
	}

	encoding, err := call.Args().Encoding()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "extract: encoding", err))
	}
	server.encoding = encoding

	if encoding == "json" || encoding == "text" || encoding == "json-text" || encoding == "uint64" {
		return server.project(payload)
	}

	if encoding != "" && encoding != "number" {
		return errnie.Error(errnie.Err(errnie.Validation, "extract: unsupported encoding", nil))
	}
	value, found, err := readPath(payload, server.path)

	if err != nil {
		return err
	}

	server.out = value
	server.found = found

	return nil
}

/*
Done emits the extracted value and whether the path was present.
*/
func (server *ExtractServer) Done(ctx context.Context, call Extract_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.extract.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetFound(server.found)

	results.SetMissing()

	if server.found {
		switch server.encoding {
		case "json":
			err = results.SetJson(server.raw)
		case "text", "json-text":
			err = results.SetText(server.text)
		case "uint64":
			results.SetUnsigned(server.unsigned)
		default:
			results.SetOut(server.out)
		}

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "extract: output", err))
		}
	}

	server.found = false
	server.out, server.raw, server.text = 0, nil, ""
	server.unsigned = 0
	return nil
}

/*
readPath resolves a dotted path against a JSON structure, and is shared by
every primitive that addresses a value by path.
*/
func readPath(payload []byte, path string) (float64, bool, error) {
	var document any

	if err := sonic.Unmarshal(payload, &document); err != nil {
		return 0, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"[data] payload is not a structure",
			err,
		))
	}

	value, found := extractAt(document, strings.Split(path, "."))
	return value, found, nil
}

/*
extractAt walks segments through the decoded structure.
*/
func extractAt(document any, segments []string) (float64, bool) {
	current := document

	for _, segment := range segments {
		index, indexed := arrayIndex(segment)

		if indexed {
			elements, ok := current.([]any)

			if !ok || index >= len(elements) {
				return 0, false
			}

			current = elements[index]
			continue
		}

		object, ok := current.(map[string]any)

		if !ok {
			return 0, false
		}

		current, ok = object[segment]

		if !ok {
			return 0, false
		}
	}

	return numeric(current)
}

/*
numeric reads a terminal value as a number, accepting the strings venues use
to carry exact decimals.
*/
func numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)

		if err != nil {
			return 0, false
		}

		return parsed, true
	case bool:
		if typed {
			return 1, true
		}

		return 0, true
	}

	return 0, false
}

/* project preserves structured values and exact integers at a declared JSON path. */
func (server *ExtractServer) project(payload []byte) error {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	if err := decoder.Decode(&document); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "extract: JSON document", err))
	}
	value, found := walk(document, strings.Split(server.path, "."))
	server.found = found

	if !found {
		return nil
	}

	if server.encoding == "uint64" {
		number, valid := value.(json.Number)

		if !valid {
			return errnie.Error(errnie.Err(errnie.Validation, "extract: selected value is not an unsigned integer", nil))
		}
		unsigned, err := strconv.ParseUint(number.String(), 10, 64)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "extract: selected value is not an unsigned integer", err))
		}
		server.unsigned = unsigned
		return nil
	}

	if server.encoding == "text" {
		text, valid := value.(string)

		if !valid {
			return errnie.Error(errnie.Err(errnie.Validation, "extract: selected value is not text", nil))
		}
		server.text = text
		return nil
	}
	raw, err := json.Marshal(value)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "extract: encode value", err))
	}
	server.raw = raw

	if server.encoding == "json-text" {
		server.text = string(raw)
	}

	return nil
}
