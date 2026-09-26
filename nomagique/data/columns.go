package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
ColumnsServer zips named columns into rows.
*/
type ColumnsServer struct {
	out []byte
}

func NewColumns() *ColumnsServer {
	return &ColumnsServer{}
}

func (server *ColumnsServer) Write(ctx context.Context, call Columns_write) error {
	args := call.Args()
	server.out = nil

	names, err := args.Names()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.columns: failed to read names", err))
	}

	index, err := args.Index()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.columns: failed to read index", err))
	}

	columns, err := args.Columns()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.columns: failed to read columns", err))
	}

	fields := strings.Split(names, ",")

	if columns.Len() != len(fields) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("data.columns: %d names for %d columns", len(fields), columns.Len()),
			nil,
		))
	}

	decoded := make([][]json.RawMessage, len(fields))

	for position := range columns.Len() {
		payload, err := columns.At(position)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.columns: failed to read a column", err))
		}

		// A column that has not arrived leaves no rows to make.
		if len(payload) == 0 {
			return nil
		}

		if err := json.Unmarshal(payload, &decoded[position]); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.columns: column "+fields[position]+" is not a JSON array", err))
		}

		if len(decoded[position]) == 0 {
			return nil
		}

		if len(decoded[position]) != len(decoded[0]) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("data.columns: column %s has %d elements, %s has %d",
					fields[position], len(decoded[position]), fields[0], len(decoded[0])),
				nil,
			))
		}
	}

	rows := make([]map[string]json.RawMessage, len(decoded[0]))

	for row := range rows {
		rows[row] = make(map[string]json.RawMessage, len(fields)+1)

		for position, field := range fields {
			rows[row][strings.TrimSpace(field)] = decoded[position][row]
		}

		if index != "" {
			rows[row][index] = json.RawMessage(fmt.Sprint(row))
		}
	}

	encoded, err := json.Marshal(rows)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.columns: failed to encode rows", err))
	}

	server.out = encoded
	return nil
}

func (server *ColumnsServer) Done(ctx context.Context, call Columns_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.columns: failed to allocate results", err))
	}

	results.SetIdle()

	if server.out != nil {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "data.columns: failed to set out", err))
		}
	}

	server.out = nil
	return nil
}
