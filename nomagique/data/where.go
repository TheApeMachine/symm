package data

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
WhereServer keeps the documents whose value at a path equals a given value.
*/
type WhereServer struct {
	kept [][]byte
}

func NewWhere() *WhereServer {
	return &WhereServer{}
}

func (server *WhereServer) Write(ctx context.Context, call Where_write) error {
	call.Args().Message().ResetReadLimit(math.MaxUint64)
	documents, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.where: failed to read data", err))
	}

	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.where: failed to read path", err))
	}

	raw, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.where: failed to read value", err))
	}

	if path == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "data.where: path is not defined", nil))
	}

	wanted, err := decodeDocument(raw)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.where: value is not JSON", err))
	}

	server.kept = server.kept[:0]
	segments := strings.Split(path, ".")

	for index := range documents.Len() {
		payload, err := documents.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.where: failed to read a document", err))
		}

		document, err := decodeDocument(payload)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.where: a document is not JSON", err))
		}

		value, found := walk(document, segments)

		if !found || !reflect.DeepEqual(value, wanted) {
			continue
		}

		server.kept = append(server.kept, bytes.Clone(payload))
	}

	return nil
}

func (server *WhereServer) Done(ctx context.Context, call Where_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.where: failed to allocate results", err))
	}

	defer func() {
		server.kept = server.kept[:0]
	}()

	if len(server.kept) == 0 {
		results.SetIdle()
		return nil
	}

	out, err := results.NewOut(int32(len(server.kept)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.where: failed to allocate out", err))
	}

	for index, document := range server.kept {
		if err := out.Set(index, document); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "data.where: failed to set a document", err))
		}
	}

	return nil
}

/*
decodeDocument reads JSON keeping numbers as written, so equal numbers compare
equal however large they are.
*/
func decodeDocument(payload []byte) (any, error) {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}

	return document, nil
}
