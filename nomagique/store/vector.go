package store

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
VectorServer retains numeric records of a fixed width, addressed by position.
*/
type VectorServer struct {
	scope     string
	width     int
	values    []float64
	written   []bool
	request   []int64
	requested bool
}

func NewVector() *VectorServer {
	return &VectorServer{}
}

/*
Write remembers which records the next done hands back and stores any
records written.
*/
func (server *VectorServer) Write(ctx context.Context, call Vector_write) error {
	args := call.Args()
	width := int(args.Width())

	if width == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"store.vector: width must be declared",
			nil,
		))
	}

	if server.width != 0 && server.width != width {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("store.vector: width changed from %d to %d", server.width, width),
			nil,
		))
	}

	server.width = width

	if err := server.enter(args); err != nil {
		return err
	}

	if args.HasRead() {
		read, err := args.Read()

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read read", err))
		}

		server.request = server.request[:0]

		for position := range read.Len() {
			server.request = append(server.request, read.At(position))
		}

		server.requested = true
	}

	index, err := args.Index()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read index", err))
	}

	values, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read values", err))
	}

	if values.Len() != index.Len()*width {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("store.vector: %d indices of width %d need %d values, got %d",
				index.Len(), width, index.Len()*width, values.Len()),
			nil,
		))
	}

	for position := range index.Len() {
		record := int(index.At(position))

		if record < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("store.vector: negative record %d", record),
				nil,
			))
		}

		server.grow(record + 1)

		for offset := range width {
			server.values[record*width+offset] = values.At(position*width + offset)
		}

		server.written[record] = true
	}

	return nil
}

/*
enter moves the vector to the series its records belong to. A new series
starts from nothing.
*/
func (server *VectorServer) enter(args Vector_write_Params) error {
	scopes, err := args.Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read scope", err))
	}

	if scopes.Len() == 0 {
		return nil
	}

	scope, err := scopes.At(0)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read a scope", err))
	}

	for index := 1; index < scopes.Len(); index++ {
		other, err := scopes.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "store.vector: failed to read a scope", err))
		}

		if other != scope {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"store.vector: records written together belong to different scopes: "+scope+", "+other,
				nil,
			))
		}
	}

	if scope == server.scope {
		return nil
	}

	server.scope = scope
	server.values = server.values[:0]
	server.written = server.written[:0]
	return nil
}

func (server *VectorServer) grow(records int) {
	if records <= len(server.written) {
		return
	}

	server.values = append(server.values, make([]float64, (records-len(server.written))*server.width)...)
	server.written = append(server.written, make([]bool, records-len(server.written))...)
}

/*
Done hands back the requested records, or the whole vector when none were
requested, as they stood before this evaluation's feedback.
*/
func (server *VectorServer) Done(ctx context.Context, call Vector_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.vector: failed to allocate results", err))
	}

	results.SetRecords(int64(len(server.written)))

	records := server.request

	if !server.requested {
		records = make([]int64, len(server.written))

		for record := range records {
			records[record] = int64(record)
		}
	}

	server.requested = false

	values, err := results.NewValues(int32(len(records) * server.width))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.vector: failed to allocate values", err))
	}

	found, err := results.NewFound(int32(len(records)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.vector: failed to allocate found", err))
	}

	for position, record := range records {
		if record < 0 || int(record) >= len(server.written) || !server.written[record] {
			continue
		}

		found.Set(position, true)

		for offset := range server.width {
			values.Set(position*server.width+offset, server.values[int(record)*server.width+offset])
		}
	}

	return nil
}
