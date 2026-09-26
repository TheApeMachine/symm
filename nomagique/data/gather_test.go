package data_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGatherWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given numbers gathered from three producers", t, func() {
		client := data.Gather_ServerToClient(data.NewGather())
		defer client.Release()

		gather := func(values []float64, present []bool) ([]float64, []bool) {
			So(client.Write(ctx, func(params data.Gather_write_Params) error {
				numbers, err := params.NewValues(int32(len(values)))

				if err != nil {
					return err
				}

				flags, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for slot := range values {
					numbers.Set(slot, values[slot])
					flags.Set(slot, present[slot])
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, delivered := []float64{}, []bool{}

			if results.Which() == data.Gathered_Which_idle {
				return out, delivered
			}

			numbers, err := results.Gathered().Values()
			So(err, ShouldBeNil)
			flags, err := results.Gathered().Present()
			So(err, ShouldBeNil)

			for slot := range numbers.Len() {
				out = append(out, numbers.At(slot))
				delivered = append(delivered, flags.At(slot))
			}

			return out, delivered
		}

		Convey("The cut stays blocked until every metric has initialized", func() {
			values, present := gather([]float64{1, 0, 3}, []bool{true, false, true})
			So(values, ShouldBeEmpty)
			So(present, ShouldBeEmpty)

			Convey("Initialization completes the cut and later arrivals retain other metrics", func() {
				values, present := gather([]float64{0, 2, 0}, []bool{false, true, false})
				So(values, ShouldResemble, []float64{1, 2, 3})
				So(present, ShouldResemble, []bool{true, true, true})
				values, present = gather([]float64{4, 0, 0}, []bool{true, false, false})
				So(values, ShouldResemble, []float64{4, 2, 3})
				So(present, ShouldResemble, []bool{true, true, true})
			})

			Convey("And nothing arriving is no list at all", func() {
				values, present := gather(nil, nil)
				So(values, ShouldBeEmpty)
				So(present, ShouldBeEmpty)
			})

			Convey("And slots none of whose producers delivered are idle", func() {
				values, present := gather([]float64{0, 0, 0}, []bool{false, false, false})
				So(values, ShouldBeEmpty)
				So(present, ShouldBeEmpty)
			})
		})

		Convey("Presence that does not match the values is rejected", func() {
			So(client.Write(ctx, func(params data.Gather_write_Params) error {
				_, err := params.NewValues(2)
				return err
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})

	Convey("Given numbers gathered with declared signal families", t, func() {
		client := data.Gather_ServerToClient(data.NewGather())
		defer client.Release()

		familiesJSON := `[{"name":"ticker_fam","count":2},{"name":"trade_fam","count":2}]`

		step := func(values []float64, present []bool) (bool, string, map[string]any) {
			So(client.Write(ctx, func(params data.Gather_write_Params) error {
				numbers, err := params.NewValues(int32(len(values)))
				So(err, ShouldBeNil)
				flags, err := params.NewPresent(int32(len(present)))
				So(err, ShouldBeNil)

				for slot := range values {
					numbers.Set(slot, values[slot])
					flags.Set(slot, present[slot])
				}

				So(params.SetFamilies(familiesJSON), ShouldBeNil)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			phase, err := results.Phase()
			So(err, ShouldBeNil)

			readinessBytes, err := results.Readiness()
			So(err, ShouldBeNil)

			var meta map[string]any
			So(json.Unmarshal(readinessBytes, &meta), ShouldBeNil)

			isGathered := results.Which() == data.Gathered_Which_gathered
			return isGathered, phase, meta
		}

		Convey("One metric per family does not admit a biased partial cut", func() {
			gathered, _, meta := step([]float64{10, 0, 30, 0}, []bool{true, false, true, false})
			So(gathered, ShouldBeFalse)
			So(meta["ready"], ShouldEqual, false)
			So(meta["contributing"], ShouldEqual, 0)
			gathered, _, _ = step([]float64{0, 20, 0, 40}, []bool{false, true, false, true})
			So(gathered, ShouldBeTrue)
		})

		Convey("Produces output once all declared families have contributed", func() {
			// Step 1: ticker_fam contributes
			gathered, phase, meta := step([]float64{10, 20, 0, 0}, []bool{true, true, false, false})
			So(gathered, ShouldBeFalse)
			So(phase, ShouldEqual, "WARMING_SIGNALS")
			So(meta["ready"], ShouldEqual, false)
			So(meta["contributing"], ShouldEqual, 1)
			So(meta["total"], ShouldEqual, 2)
			So(meta["missing"], ShouldResemble, []any{"trade_fam"})

			// Step 2: trade_fam contributes, completing all families
			gathered, phase, meta = step([]float64{0, 0, 30, 40}, []bool{false, false, true, true})
			So(gathered, ShouldBeTrue)
			So(phase, ShouldEqual, "FORMING_MAP")
			So(meta["ready"], ShouldEqual, true)
			So(meta["contributing"], ShouldEqual, 2)
			So(meta["missing"], ShouldBeEmpty)

			// Step 3: asynchronous arrival with only ticker present produces output and stays ready
			gathered, phase, meta = step([]float64{15, 25, 0, 0}, []bool{true, true, false, false})
			So(gathered, ShouldBeTrue)
			So(phase, ShouldEqual, "FORMING_MAP")
			So(meta["ready"], ShouldEqual, true)
			So(meta["contributing"], ShouldEqual, 2)
			So(meta["missing"], ShouldBeEmpty)
		})
	})
}

/* BenchmarkGatherWrite uses the 411-coordinate universe declared by cut.json. */
func BenchmarkGatherWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Gather_ServerToClient(data.NewGather())
	defer client.Release()
	b.ReportAllocs()

	iteration := 0
	for b.Loop() {
		err := client.Write(ctx, func(params data.Gather_write_Params) error {
			params.SetEpoch(1)
			params.SetSequence(int64(iteration))
			if err := params.SetScope("BTC/USD"); err != nil {
				return err
			}
			identities, err := params.NewIdentities(411)
			if err != nil {
				return err
			}
			for index := range 411 {
				if err := identities.Set(index, fmt.Sprintf("metric-%d", index)); err != nil {
					return err
				}
			}

			values, err := params.NewValues(411)

			if err != nil {
				return err
			}
			present, err := params.NewPresent(411)

			if err != nil {
				return err
			}

			for index := range 411 {
				values.Set(index, float64(iteration+index))
				present.Set(index, iteration == 0 || index%3 == iteration%3)
			}
			return nil
		})

		if err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		iteration++
		future, release := client.Done(ctx, nil)
		_, err = future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestGatherWriteStamped(t *testing.T) {
	Convey("Causal cuts preserve each coordinate's stamp across quiet observations", t, func() {
		client := data.Gather_ServerToClient(data.NewGather())
		defer client.Release()
		for _, observation := range []struct {
			epoch, sequence int64
			scope           string
			slot            int
			value           float64
			ready           bool
			stamps          []int64
		}{
			{91, 0, "BTC", 0, 10, false, nil},
			{91, 1, "BTC", 1, 20, true, []int64{0, 1}},
			{91, 2, "BTC", 0, 30, true, []int64{2, 1}},
			{91, 3, "ETH", 1, 40, false, nil},
			{91, 4, "ETH", 0, 50, true, []int64{4, 3}},
			{92, 0, "ETH", 0, 60, false, nil},
			{92, 1, "ETH", 1, 70, true, []int64{0, 1}},
		} {
			So(client.Write(context.Background(), func(args data.Gather_write_Params) error {
				args.SetEpoch(observation.epoch)
				args.SetSequence(observation.sequence)
				if err := args.SetScope(observation.scope); err != nil {
					return err
				}
				values, err := args.NewValues(2)
				if err != nil {
					return err
				}
				present, err := args.NewPresent(2)
				if err != nil {
					return err
				}
				values.Set(observation.slot, observation.value)
				present.Set(observation.slot, true)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which() == data.Gathered_Which_gathered, ShouldEqual, observation.ready)
			if observation.ready {
				So(result.Epoch(), ShouldEqual, observation.epoch)
				So(result.Sequence(), ShouldEqual, observation.sequence)
				stamps, err := result.Sequences()
				So(err, ShouldBeNil)
				So([]int64{stamps.At(0), stamps.At(1)}, ShouldResemble, observation.stamps)
				values, err := result.Gathered().Values()
				So(err, ShouldBeNil)
				So(values.At(observation.slot), ShouldEqual, observation.value)
			}
			release()
		}
	})
}

func TestGatherWriteReplay(t *testing.T) {
	Convey("Stored metric cuts replay with their own stamps and no raw signal calculation", t, func() {
		for _, fixture := range []struct {
			row   string
			valid bool
		}{
			{`{"epoch":1790000000000000001,"sequence":42,"symbol":"BTC/USD","complete":true,"metrics":[{"identity":"ticker:spread","value":5,"present":true,"epoch":1790000000000000001,"sequence":40},{"identity":"trade:count","value":17,"present":true,"epoch":1790000000000000001,"sequence":42}]}`, true},
			{`{"epoch":91,"sequence":42,"symbol":"BTC/USD","complete":true,"metrics":[{"identity":"ticker:spread","value":5,"present":true,"epoch":91,"sequence":43},{"identity":"trade:count","value":17,"present":true,"epoch":91,"sequence":42}]}`, false},
			{`{"epoch":91,"sequence":42,"symbol":"BTC/USD","complete":true,"metrics":[{"identity":"trade:count","value":5,"present":true,"epoch":91,"sequence":40},{"identity":"ticker:spread","value":17,"present":true,"epoch":91,"sequence":42}]}`, false},
		} {
			client := data.Gather_ServerToClient(data.NewGather())
			err := client.Write(context.Background(), func(args data.Gather_write_Params) error {
				names, err := args.NewIdentities(2)
				if err != nil {
					return err
				}
				if err := names.Set(0, "ticker:spread"); err != nil {
					return err
				}
				if err := names.Set(1, "trade:count"); err != nil {
					return err
				}
				return args.SetRow([]byte(fixture.row))
			})
			So(err, ShouldBeNil)
			err = client.WaitStreaming()
			if !fixture.valid {
				So(err, ShouldNotBeNil)
				client.Release()
				continue
			}
			So(err, ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Epoch(), ShouldEqual, int64(1790000000000000001))
			So(result.Sequence(), ShouldEqual, 42)
			values, err := result.Gathered().Values()
			So(err, ShouldBeNil)
			So([]float64{values.At(0), values.At(1)}, ShouldResemble, []float64{5, 17})
			stamps, err := result.Sequences()
			So(err, ShouldBeNil)
			So([]int64{stamps.At(0), stamps.At(1)}, ShouldResemble, []int64{40, 42})
			release()
			client.Release()
		}
	})
}

func TestGatherWriteProvenance(t *testing.T) {
	Convey("Source frame and record identity remains exact through metric persistence and replay", t, func() {
		for _, observation := range []string{
			`{"capture":{"session":"spot-connection","sequence":9007199254740993,"record":2,"endpoint":"wss://fixture","receivedAt":"2026-09-26T12:00:00Z"}}`,
			`{}`,
			`{"capture":{"session":"spot","sequence":1,"endpoint":"wss://fixture","receivedAt":"2026-09-26T12:00:00Z"}}`,
		} {
			client := data.Gather_ServerToClient(data.NewGather())
			So(client.Write(context.Background(), func(args data.Gather_write_Params) error {
				args.SetEpoch(91)
				args.SetSequence(17)
				args.SetRequireProvenance(true)
				if err := args.SetScope("BTC/USD"); err != nil {
					return err
				}
				if err := args.SetObservation([]byte(observation)); err != nil {
					return err
				}
				names, err := args.NewIdentities(1)
				if err != nil {
					return err
				}
				if err := names.Set(0, "spread"); err != nil {
					return err
				}
				values, err := args.NewValues(1)
				if err != nil {
					return err
				}
				values.Set(0, 5)
				present, err := args.NewPresent(1)
				if err != nil {
					return err
				}
				present.Set(0, true)
				return nil
			}), ShouldBeNil)
			err := client.WaitStreaming()
			if observation != `{"capture":{"session":"spot-connection","sequence":9007199254740993,"record":2,"endpoint":"wss://fixture","receivedAt":"2026-09-26T12:00:00Z"}}` {
				So(err, ShouldNotBeNil)
				client.Release()
				continue
			}
			So(err, ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			row, err := result.Row()
			So(err, ShouldBeNil)
			So(row.TypeId(), ShouldEqual, uint64(data.MetricCut_TypeID))
			pointer, err := row.Value()
			So(err, ShouldBeNil)
			cut := data.MetricCut(pointer.Struct())
			provenance, err := cut.Provenance()
			So(err, ShouldBeNil)
			So(provenance, ShouldContainSubstring, "9007199254740993")
			replay := data.Gather_ServerToClient(data.NewGather())
			So(replay.Write(context.Background(), func(args data.Gather_write_Params) error {
				names, err := args.NewIdentities(1)
				if err != nil {
					return err
				}
				if err := names.Set(0, "spread"); err != nil {
					return err
				}
				encoded, err := json.Marshal(map[string]any{"epoch": cut.Epoch(), "sequence": cut.Sequence(), "symbol": "BTC/USD", "complete": true, "provenance": provenance, "metrics": []any{map[string]any{"identity": "spread", "value": 5, "present": true, "epoch": cut.Epoch(), "sequence": cut.Sequence()}}})
				if err != nil {
					return err
				}
				return args.SetRow(encoded)
			}), ShouldBeNil)
			So(replay.WaitStreaming(), ShouldBeNil)
			read, done := replay.Done(context.Background(), nil)
			recovered, err := read.Struct()
			So(err, ShouldBeNil)
			restored, err := recovered.Row()
			So(err, ShouldBeNil)
			restoredPointer, err := restored.Value()
			So(err, ShouldBeNil)
			restoredCut := data.MetricCut(restoredPointer.Struct())
			So(restoredCut.Epoch(), ShouldEqual, cut.Epoch())
			So(restoredCut.Sequence(), ShouldEqual, cut.Sequence())
			restoredProvenance, err := restoredCut.Provenance()
			So(err, ShouldBeNil)
			So(restoredProvenance, ShouldEqual, provenance)
			done()
			release()
			replay.Release()
			client.Release()
		}
	})
}
