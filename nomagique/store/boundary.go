package store

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/data"
)

type boundarySeries struct {
	sequence int64
	layout   string
	covered  []bool
}

type boundaryFrame struct {
	run        string
	sequence   int64
	row        []byte
	payload    []byte
	receipt    []byte
	projection boundaryProjection
	covered    int
}

// BoundaryServer retains only lineage and bootstrap coverage. The sealed
// record itself is sent to explicit storage, never kept as growing history.
type BoundaryServer struct {
	series map[string]*boundarySeries
	frame  *boundaryFrame
}

func NewBoundary() *BoundaryServer {
	return &BoundaryServer{series: make(map[string]*boundarySeries)}
}

func (server *BoundaryServer) Write(ctx context.Context, call Boundary_write) error {
	server.frame = nil
	args := call.Args()
	replay, err := args.Replay()

	if err != nil {
		return boundaryError("read replay row", err)
	}

	if len(replay) > 0 {
		if args.Sequence() != 0 {
			return boundaryError("live and replay input cannot be mixed", nil)
		}

		return server.replay(replay)
	}

	if args.Sequence() == 0 {
		return nil
	}

	record, err := server.seal(args)

	if err != nil {
		return err
	}

	payload, err := record.Message().Marshal()

	if err != nil {
		return boundaryError("encode sealed boundary", err)
	}

	return server.accept(record, payload)
}

func (server *BoundaryServer) seal(args Boundary_write_Params) (MeasurementBoundary, error) {
	run, err := args.Run()

	if err != nil || run == "" {
		return MeasurementBoundary{}, boundaryError("run identity is required", err)
	}

	layout, err := args.Layout()

	if err != nil {
		return MeasurementBoundary{}, boundaryError("read layout", err)
	}

	receipt, err := args.Receipt()

	if err != nil || len(receipt) == 0 {
		return MeasurementBoundary{}, boundaryError("source receipt is required", err)
	}

	inputs, err := args.Publications()

	if err != nil {
		return MeasurementBoundary{}, boundaryError("read input publications", err)
	}

	measurements := make([]data.Measurement, 0, inputs.Len())

	for index := range inputs.Len() {
		encoded, err := inputs.At(index)

		if err != nil || len(encoded) == 0 {
			return MeasurementBoundary{}, boundaryError("a declared producer did not publish", err)
		}

		message, err := capnp.Unmarshal(encoded)

		if err != nil {
			return MeasurementBoundary{}, boundaryError("decode publication", err)
		}

		measurement, err := data.ReadRootMeasurement(message)

		if err != nil {
			return MeasurementBoundary{}, boundaryError("read publication", err)
		}

		measurements = append(measurements, measurement)
	}

	// Canonical ordering affects storage only; grid identity remains coordinate.
	owners := make(map[data.Measurement]string, len(measurements))

	for _, measurement := range measurements {
		owner, err := measurement.Producer()

		if err != nil {
			return MeasurementBoundary{}, boundaryError("read owner", err)
		}

		owners[measurement] = owner
	}

	slices.SortFunc(measurements, func(left, right data.Measurement) int {
		return bytes.Compare([]byte(owners[left]), []byte(owners[right]))
	})

	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return MeasurementBoundary{}, boundaryError("allocate boundary", err)
	}

	record, err := NewRootMeasurementBoundary(segment)

	if err != nil {
		return MeasurementBoundary{}, boundaryError("allocate boundary record", err)
	}

	record.SetVersion(1)
	record.SetSequence(args.Sequence())

	if series := server.series[run]; series != nil {
		record.SetPrevious(series.sequence)
	}

	if err := record.SetRun(run); err != nil {
		return MeasurementBoundary{}, boundaryError("set run", err)
	}

	if err := record.SetLayout(layout); err != nil {
		return MeasurementBoundary{}, boundaryError("set layout", err)
	}

	if err := record.SetReceipt(receipt); err != nil {
		return MeasurementBoundary{}, boundaryError("set receipt", err)
	}

	publications, err := record.NewPublications(int32(len(measurements)))

	if err != nil {
		return MeasurementBoundary{}, boundaryError("allocate publication list", err)
	}

	for index, measurement := range measurements {
		if err := publications.Set(index, measurement); err != nil {
			return MeasurementBoundary{}, boundaryError("copy publication", err)
		}
	}

	return record, nil
}

func (server *BoundaryServer) replay(encoded []byte) error {
	var row BoundaryRow

	if err := json.Unmarshal(encoded, &row); err != nil {
		return boundaryError("decode archive row", err)
	}

	if len(row.Payload) == 0 || row.Digest != boundaryDigest(row.Payload) {
		return boundaryError("archive payload digest differs", nil)
	}

	record, err := readBoundary(row.Payload)

	if err != nil {
		return err
	}

	run, err := record.Run()

	if err != nil || run != row.Run || record.Sequence() != row.Sequence || record.Previous() != row.Previous {
		return boundaryError("archive index differs from sealed identity", err)
	}

	return server.accept(record, bytes.Clone(row.Payload))
}

func (server *BoundaryServer) accept(record MeasurementBoundary, payload []byte) error {
	projection, err := projectBoundary(record)

	if err != nil {
		return err
	}

	run, err := record.Run()

	if err != nil {
		return boundaryError("read run", err)
	}

	series := server.series[run]

	if series == nil {
		series = &boundarySeries{layout: projection.layout, covered: make([]bool, len(projection.values))}
	}

	if record.Previous() != series.sequence || record.Sequence() <= series.sequence {
		return boundaryError("missing preceding boundary, duplicate, or out-of-order sequence", nil)
	}

	if series.layout != projection.layout {
		return boundaryError("coordinate universe changed inside a run; a new run is required", nil)
	}

	receipt, err := record.Receipt()

	if err != nil || len(receipt) == 0 {
		return boundaryError("sealed source receipt is missing", err)
	}

	row, err := json.Marshal(BoundaryRow{Run: run, Sequence: record.Sequence(), Previous: record.Previous(), Digest: boundaryDigest(payload), Payload: payload})

	if err != nil {
		return boundaryError("encode archive envelope", err)
	}

	covered := 0

	for index, present := range projection.present {
		series.covered[index] = series.covered[index] || present

		if series.covered[index] {
			covered++
		}
	}

	series.sequence = record.Sequence()
	server.series[run] = series
	server.frame = &boundaryFrame{run: run, sequence: record.Sequence(), row: row, payload: payload, receipt: bytes.Clone(receipt), projection: projection, covered: covered}
	return nil
}

func (server *BoundaryServer) Done(ctx context.Context, call Boundary_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return boundaryError("allocate results", err)
	}

	frame := server.frame
	server.frame = nil

	if frame == nil {
		results.SetIdle()
		return nil
	}

	if err := results.SetRow(frame.row); err != nil {
		return boundaryError("publish archive row", err)
	}

	if err := results.SetPayload(frame.payload); err != nil {
		return boundaryError("publish binary boundary", err)
	}

	if err := results.SetRun(frame.run); err != nil {
		return boundaryError("publish run", err)
	}

	results.SetSequence(frame.sequence)

	if err := results.SetReceipt(frame.receipt); err != nil {
		return boundaryError("publish source receipt", err)
	}

	ready := frame.covered == len(frame.projection.values)
	phase := "WARMING_MEASUREMENTS"

	if ready {
		phase = "MEASUREMENTS_READY"
	}

	readiness, err := json.Marshal(struct {
		Phase string `json:"phase"`
		Ready bool `json:"ready"`
		Covered int `json:"covered"`
		Total int `json:"total"`
		Sequence int64 `json:"sequence"`
	}{phase, ready, frame.covered, len(frame.projection.values), frame.sequence})

	if err != nil {
		return boundaryError("encode readiness", err)
	}

	if err := results.SetReadiness(readiness); err != nil {
		return boundaryError("publish readiness", err)
	}

	if !ready {
		results.SetWarming()
		return nil
	}

	results.SetReady()
	values, err := results.Ready().NewValues(int32(len(frame.projection.values)))

	if err != nil {
		return boundaryError("allocate coordinate values", err)
	}

	present, err := results.Ready().NewPresent(int32(len(frame.projection.present)))

	if err != nil {
		return boundaryError("allocate coordinate presence", err)
	}

	for index, value := range frame.projection.values {
		values.Set(index, value)
		present.Set(index, frame.projection.present[index])
	}

	return nil
}
