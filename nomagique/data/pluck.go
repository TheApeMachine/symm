package data

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
PluckServer collects one field from each of a list of documents.
*/
type PluckServer struct {
	out []byte
}

func NewPluck() *PluckServer {
	return &PluckServer{}
}

func (server *PluckServer) Write(ctx context.Context, call Pluck_write) error {
	server.out = nil
	call.Args().Message().ResetReadLimit(math.MaxUint64)
	documents, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.pluck: failed to read data", err))
	}

	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.pluck: failed to read path", err))
	}

	if path == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "data.pluck: path is not defined", nil))
	}

	segments := strings.Split(path, ".")
	plucked := make([]any, 0, documents.Len())

	for index := range documents.Len() {
		payload, err := documents.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.pluck: failed to read a document", err))
		}

		document, err := decodeDocument(payload)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.pluck: a document is not JSON", err))
		}

		if value, found := walk(document, segments); found {
			plucked = append(plucked, value)
		}
	}

	if len(plucked) == 0 {
		return nil
	}

	encoded, err := json.Marshal(plucked)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.pluck: failed to encode out", err))
	}

	server.out = encoded
	return nil
}

func (server *PluckServer) Done(ctx context.Context, call Pluck_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.pluck: failed to allocate results", err))
	}

	defer func() {
		server.out = nil
	}()

	if len(server.out) == 0 {
		results.SetIdle()
		return nil
	}

	return results.SetOut(server.out)
}
