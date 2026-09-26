package store

import (
	"context"
	"fmt"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	runtime "github.com/theapemachine/symm/nomagique/runtime"
)

/*
VectorServer retains numeric records of a fixed width, addressed by position.
*/
type VectorServer struct {
	*vectorSeries
	series    map[string]*vectorSeries
	scope     string
	width     int
	request   []int64
	requested bool
}

/* vectorSeries owns the records retained under one native scope. */
type vectorSeries struct {
	values  []float64
	written []bool
}

func NewVector() *VectorServer {
	series := &vectorSeries{}
	return &VectorServer{vectorSeries: series, series: map[string]*vectorSeries{"": series}}
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
enter selects the series its records belong to, resuming its retained records.
A previously unseen series starts from nothing.
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

	series, found := server.series[scope]

	if !found {
		series = &vectorSeries{}
		server.series[scope] = series
	}
	server.vectorSeries, server.scope = series, scope
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

/* Snapshot serializes the retained vector, excluding transient read requests. */
func (server *VectorServer) Snapshot(ctx context.Context, call runtime.Snapshot_snapshot) error {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	snapshot, err := NewRootVectorSnapshot(segment)
	if err != nil {
		return errnie.Error(err)
	}

	if err := server.vectorSeries.snapshot(snapshot, server.scope, server.width); err != nil {
		return err
	}
	keys := make([]string, 0, len(server.series))
	for scope := range server.series {
		if scope != server.scope {
			keys = append(keys, scope)
		}
	}
	slices.Sort(keys)
	others, err := snapshot.NewOthers(int32(len(keys)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, scope := range keys {
		if err := server.series[scope].snapshot(others.At(index), scope, server.width); err != nil {
			return err
		}
	}
	encoded, err := message.Marshal()
	if err != nil {
		return errnie.Error(err)
	}
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetData(encoded))
}

/* Restore validates the entire vector before replacing its retained state. */
func (server *VectorServer) Restore(ctx context.Context, call runtime.Snapshot_restore) error {
	encoded, err := call.Args().Data()
	if err != nil {
		return errnie.Error(err)
	}
	message, err := capnp.Unmarshal(encoded)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "vector: invalid snapshot", err))
	}
	defer message.Release()
	snapshot, err := ReadRootVectorSnapshot(message)
	if err != nil {
		return errnie.Error(err)
	}

	width := int(snapshot.Width())
	if server.width != 0 && server.width != width {
		return errnie.Error(errnie.Err(errnie.Validation, "vector: snapshot width differs", nil))
	}
	scope, err := snapshot.Scope()
	if err != nil {
		return errnie.Error(err)
	}
	active, err := restoreVectorSeries(snapshot, width)
	if err != nil {
		return err
	}
	restored := map[string]*vectorSeries{scope: active}
	others, err := snapshot.Others()
	if err != nil {
		return errnie.Error(err)
	}
	for index := range others.Len() {
		item := others.At(index)
		name, err := item.Scope()
		if err != nil {
			return errnie.Error(err)
		}
		if _, found := restored[name]; found {
			return errnie.Error(errnie.Err(errnie.Validation, "vector: duplicate snapshot scope", nil))
		}
		nested, err := item.Others()
		if err != nil {
			return errnie.Error(err)
		}
		if nested.Len() != 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "vector: nested snapshot scopes", nil))
		}
		series, err := restoreVectorSeries(item, width)
		if err != nil {
			return err
		}
		restored[name] = series
	}
	server.vectorSeries, server.series, server.width, server.scope = active, restored, width, scope
	server.request, server.requested = nil, false
	return nil
}

/* snapshot writes one series in the existing native record representation. */
func (series *vectorSeries) snapshot(snapshot VectorSnapshot, scope string, width int) error {
	snapshot.SetWidth(uint32(width))
	if err := snapshot.SetScope(scope); err != nil {
		return errnie.Error(err)
	}
	values, err := snapshot.NewValues(int32(len(series.values)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range series.values {
		values.Set(index, value)
	}
	written, err := snapshot.NewWritten(int32(len(series.written)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range series.written {
		written.Set(index, value)
	}
	return nil
}

/* restoreVectorSeries validates and copies one owned series before installation. */
func restoreVectorSeries(snapshot VectorSnapshot, width int) (*vectorSeries, error) {
	values, err := snapshot.Values()
	if err != nil {
		return nil, errnie.Error(err)
	}
	written, err := snapshot.Written()
	if err != nil {
		return nil, errnie.Error(err)
	}
	if int(snapshot.Width()) != width || values.Len() != written.Len()*width || width == 0 && written.Len() != 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "vector: snapshot dimensions differ", nil))
	}
	series := &vectorSeries{values: make([]float64, values.Len()), written: make([]bool, written.Len())}
	for index := range values.Len() {
		series.values[index] = values.At(index)
	}
	for index := range written.Len() {
		series.written[index] = written.At(index)
	}
	return series, nil
}
