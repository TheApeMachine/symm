package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

const boundaryLayout = `[{"producer":"signal","coordinates":[0]},{"producer":"logic","coordinates":[1]}]`

type boundaryObservation struct {
	row []byte
	payload []byte
	values []float64
	present []bool
	ready bool
}

func boundaryPublication(t *testing.T, owner, run string, sequence int64, coordinate uint32, value float64, present bool) []byte {
	t.Helper()
	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil { t.Fatal(err) }
	measurement, err := data.NewRootMeasurement(segment)
	if err != nil { t.Fatal(err) }
	if err := measurement.SetRun(run); err != nil { t.Fatal(err) }
	if err := measurement.SetProducer(owner); err != nil { t.Fatal(err) }
	measurement.SetTick(sequence)
	coordinates, err := measurement.NewCoordinates(1)
	if err != nil { t.Fatal(err) }
	coordinates.Set(0, coordinate)
	metrics, err := measurement.NewMetrics(1)
	if err != nil { t.Fatal(err) }
	metrics.At(0).SetRaw(value)
	flags, err := measurement.NewPresent(1)
	if err != nil { t.Fatal(err) }
	flags.Set(0, present)
	encoded, err := measurement.Message().Marshal()
	if err != nil { t.Fatal(err) }
	return encoded
}

func boundaryStep(ctx context.Context, client store.Boundary, sequence int64, publications [][]byte, replay []byte) (boundaryObservation, error) {
	var observed boundaryObservation
	err := client.Write(ctx, func(params store.Boundary_write_Params) error {
		if len(replay) > 0 { return params.SetReplay(replay) }
		params.SetSequence(sequence)
		if err := params.SetRun("recorded-run"); err != nil { return err }
		if err := params.SetLayout(boundaryLayout); err != nil { return err }
		if err := params.SetReceipt([]byte(`{"capture":"raw-source-boundary"}`)); err != nil { return err }
		inputs, err := params.NewPublications(int32(len(publications)))
		if err != nil { return err }
		for index, publication := range publications {
			if err := inputs.Set(index, publication); err != nil { return err }
		}
		return nil
	})
	if err != nil { return observed, err }
	if err := client.WaitStreaming(); err != nil { return observed, err }
	future, release := client.Done(ctx, nil)
	defer release()
	result, err := future.Struct()
	if err != nil { return observed, err }
	row, err := result.Row()
	if err != nil { return observed, err }
	observed.row = bytes.Clone(row)
	payload, err := result.Payload()
	if err != nil { return observed, err }
	observed.payload = bytes.Clone(payload)
	observed.ready = result.Which() == store.BoundaryResult_Which_ready
	if !observed.ready { return observed, nil }
	values, err := result.Ready().Values()
	if err != nil { return observed, err }
	present, err := result.Ready().Present()
	if err != nil { return observed, err }
	for index := range values.Len() {
		observed.values = append(observed.values, values.At(index))
		observed.present = append(observed.present, present.At(index))
	}
	return observed, nil
}

func TestBoundary(t *testing.T) {
	Convey("Every signal and logic publication belongs to one sealed sequence", t, func() {
		ctx := t.Context()
		client := store.Boundary_ServerToClient(store.NewBoundary())
		defer client.Release()
		pair := func(sequence int64, first, second bool) [][]byte {
			return [][]byte{
				boundaryPublication(t, "signal", "recorded-run", sequence, 0, 12, first),
				boundaryPublication(t, "logic", "recorded-run", sequence, 1, 24, second),
			}
		}

		Convey("A complete warm-up boundary is archived but cannot light the map", func() {
			first, err := boundaryStep(ctx, client, 1, pair(1, true, false), nil)
			So(err, ShouldBeNil)
			So(first.row, ShouldNotBeEmpty)
			So(first.ready, ShouldBeFalse)
			second, err := boundaryStep(ctx, client, 2, pair(2, false, true), nil)
			So(err, ShouldBeNil)
			So(second.ready, ShouldBeTrue)
			So(second.present, ShouldResemble, []bool{false, true})

			Convey("Replay uses the recorded measurements, including the warm-up boundary", func() {
				replay := store.Boundary_ServerToClient(store.NewBoundary())
				defer replay.Release()
				firstRead, err := boundaryStep(ctx, replay, 0, nil, first.row)
				So(err, ShouldBeNil)
				So(firstRead.ready, ShouldBeFalse)
				secondRead, err := boundaryStep(ctx, replay, 0, nil, second.row)
				So(err, ShouldBeNil)
				So(secondRead, ShouldResemble, second)
			})
		})

		Convey("A missing logic publication is not an undefined metric", func() {
			_, err := boundaryStep(ctx, client, 1, pair(1, true, true)[:1], nil)
			So(err, ShouldNotBeNil)
		})

		Convey("Duplicate publications cannot substitute for a missing producer", func() {
			publications := pair(1, true, true)
			publications[1] = publications[0]
			_, err := boundaryStep(ctx, client, 1, publications, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("A result from another evaluation cannot complete this one", func() {
			publications := pair(1, true, true)
			publications[1] = boundaryPublication(t, "logic", "recorded-run", 2, 1, 24, true)
			_, err := boundaryStep(ctx, client, 1, publications, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("A producer from another run cannot complete this run", func() {
			publications := pair(1, true, true)
			publications[1] = boundaryPublication(t, "logic", "another-run", 1, 1, 24, true)
			_, err := boundaryStep(ctx, client, 1, publications, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("The seal catches an entirely missing boundary", func() {
			first, err := boundaryStep(ctx, client, 1, pair(1, true, true), nil)
			So(err, ShouldBeNil)
			_, err = boundaryStep(ctx, client, 2, pair(2, true, true), nil)
			So(err, ShouldBeNil)
			third, err := boundaryStep(ctx, client, 3, pair(3, true, true), nil)
			So(err, ShouldBeNil)
			replay := store.Boundary_ServerToClient(store.NewBoundary())
			defer replay.Release()
			_, err = boundaryStep(ctx, replay, 0, nil, first.row)
			So(err, ShouldBeNil)
			_, err = boundaryStep(ctx, replay, 0, nil, third.row)
			So(err, ShouldNotBeNil)
		})

		Convey("Replaying a boundary twice is rejected rather than increasing support", func() {
			first, err := boundaryStep(ctx, client, 1, pair(1, true, true), nil)
			So(err, ShouldBeNil)
			replay := store.Boundary_ServerToClient(store.NewBoundary())
			defer replay.Release()
			_, err = boundaryStep(ctx, replay, 0, nil, first.row)
			So(err, ShouldBeNil)
			_, err = boundaryStep(ctx, replay, 0, nil, first.row)
			So(err, ShouldNotBeNil)
		})

		Convey("Archive corruption is caught before the grid can consume it", func() {
			first, err := boundaryStep(ctx, client, 1, pair(1, true, true), nil)
			So(err, ShouldBeNil)
			var row store.BoundaryRow
			So(json.Unmarshal(first.row, &row), ShouldBeNil)
			row.Payload[len(row.Payload)-1] ^= 1
			encoded, err := json.Marshal(row)
			So(err, ShouldBeNil)
			replay := store.Boundary_ServerToClient(store.NewBoundary())
			defer replay.Release()
			_, err = boundaryStep(ctx, replay, 0, nil, encoded)
			So(err, ShouldNotBeNil)
		})

		Convey("Sequence identity is exact beyond Float64 integer precision", func() {
			sequence := int64(1<<53) + 1
			observed, err := boundaryStep(ctx, client, sequence, pair(sequence, true, true), nil)
			So(err, ShouldBeNil)
			var row store.BoundaryRow
			So(json.Unmarshal(observed.row, &row), ShouldBeNil)
			So(row.Sequence, ShouldEqual, sequence)
		})

		Convey("Binary storage preserves special Float64 bit patterns without substitution", func() {
			for index, value := range []float64{math.Copysign(0, -1), math.Inf(1), math.Float64frombits(0x7ff8000000000123)} {
				sequence := int64(index + 1)
				publications := pair(sequence, true, true)
				publications[0] = boundaryPublication(t, "signal", "recorded-run", sequence, 0, value, true)
				observed, err := boundaryStep(ctx, client, sequence, publications, nil)
				So(err, ShouldBeNil)
				So(math.Float64bits(observed.values[0]), ShouldEqual, math.Float64bits(value))
			}
		})
	})
}
