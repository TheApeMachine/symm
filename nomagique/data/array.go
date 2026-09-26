package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
ArrayServer writes a list as a JSON array.
*/
type ArrayServer struct {
	out []byte
}

func NewArray() *ArrayServer {
	return &ArrayServer{}
}

func (server *ArrayServer) Write(ctx context.Context, call Array_write) error {
	args := call.Args()
	server.out = nil
	stride := max(int(args.Stride()), 1)
	offset := int(args.Offset())

	numbers, err := args.Numbers()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.array: failed to read numbers", err))
	}

	integers, err := args.Integers()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.array: failed to read integers", err))
	}

	texts, err := args.Texts()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.array: failed to read texts", err))
	}

	flags, err := args.Flags()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.array: failed to read flags", err))
	}

	// A list that did not arrive is absent, not empty: with none of them
	// delivered there is nothing to write.
	if !args.HasNumbers() && !args.HasIntegers() && !args.HasTexts() && !args.HasFlags() {
		return nil
	}

	carried := 0

	for _, length := range []int{numbers.Len(), integers.Len(), texts.Len(), flags.Len()} {
		if length > 0 {
			carried++
		}
	}

	if carried > 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("data.array: one list at a time; got %d numbers, %d integers, %d texts, %d flags",
				numbers.Len(), integers.Len(), texts.Len(), flags.Len()),
			nil,
		))
	}

	elements := []any{}

	for index := offset; index < numbers.Len(); index += stride {
		elements = append(elements, numbers.At(index))
	}

	for index := offset; index < integers.Len(); index += stride {
		elements = append(elements, integers.At(index))
	}

	for index := offset; index < texts.Len(); index += stride {
		text, err := texts.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "data.array: failed to read a text", err))
		}

		elements = append(elements, text)
	}

	for index := offset; index < flags.Len(); index += stride {
		elements = append(elements, flags.At(index))
	}

	if len(elements) == 0 {
		return nil
	}

	encoded, err := json.Marshal(elements)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.array: failed to encode", err))
	}

	server.out = encoded
	return nil
}

func (server *ArrayServer) Done(ctx context.Context, call Array_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.array: failed to allocate results", err))
	}

	if server.out == nil {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.array: failed to set out", err))
	}

	server.out = nil
	return nil
}
