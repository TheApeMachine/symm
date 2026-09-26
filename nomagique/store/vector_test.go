package store_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestVector(t *testing.T) {
	ctx := context.Background()

	Convey("Given a vector of records two numbers wide", t, func() {
		client := store.Vector_ServerToClient(store.NewVector())
		defer client.Release()

		write := func(read []int64, index []int64, values []float64) error {
			if err := client.Write(ctx, func(params store.Vector_write_Params) error {
				params.SetWidth(2)

				if read != nil {
					list, err := params.NewRead(int32(len(read)))

					if err != nil {
						return err
					}

					for position, record := range read {
						list.Set(position, record)
					}
				}

				indices, err := params.NewIndex(int32(len(index)))

				if err != nil {
					return err
				}

				for position, record := range index {
					indices.Set(position, record)
				}

				numbers, err := params.NewValues(int32(len(values)))

				if err != nil {
					return err
				}

				for position, value := range values {
					numbers.Set(position, value)
				}

				return nil
			}); err != nil {
				return err
			}

			return client.WaitStreaming()
		}

		done := func() ([]float64, []bool) {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			values, err := results.Values()
			So(err, ShouldBeNil)
			found, err := results.Found()
			So(err, ShouldBeNil)

			numbers := make([]float64, values.Len())

			for position := range values.Len() {
				numbers[position] = values.At(position)
			}

			flags := make([]bool, found.Len())

			for position := range found.Len() {
				flags[position] = found.At(position)
			}

			return numbers, flags
		}

		Convey("An empty vector hands back nothing", func() {
			values, found := done()
			So(values, ShouldBeEmpty)
			So(found, ShouldBeEmpty)
		})

		Convey("Records written stay, and records never written are unknown rather than zero", func() {
			So(write(nil, []int64{2}, []float64{3, 4}), ShouldBeNil)

			values, found := done()
			So(found, ShouldResemble, []bool{false, false, true})
			So(values, ShouldResemble, []float64{0, 0, 0, 0, 3, 4})

			Convey("A read hands back only the requested records, in request order", func() {
				So(write([]int64{2, 7}, nil, nil), ShouldBeNil)

				values, found := done()
				So(found, ShouldResemble, []bool{true, false})
				So(values, ShouldResemble, []float64{3, 4, 0, 0})

				Convey("And the request lasts one evaluation", func() {
					values, found := done()
					So(found, ShouldResemble, []bool{false, false, true})
					So(values, ShouldHaveLength, 6)
				})
			})
		})

		Convey("Records written under one scope are never handed out under another", func() {
			So(write(nil, []int64{0}, []float64{1, 2}), ShouldBeNil)

			So(client.Write(ctx, func(params store.Vector_write_Params) error {
				params.SetWidth(2)
				scopes, err := params.NewScope(1)

				if err != nil {
					return err
				}

				return scopes.Set(0, "next")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			values, found := done()
			So(values, ShouldBeEmpty)
			So(found, ShouldBeEmpty)
		})

		Convey("Values that do not fill the written records are rejected", func() {
			So(write(nil, []int64{0}, []float64{1}), ShouldNotBeNil)
		})

		Convey("A changed width is rejected", func() {
			So(write(nil, []int64{0}, []float64{1, 2}), ShouldBeNil)

			err := client.Write(ctx, func(params store.Vector_write_Params) error {
				params.SetWidth(3)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

/* TestVectorSnapshotMarkets keeps every market's previous cut across a restart. */
func TestVectorSnapshotMarkets(t *testing.T) {
	Convey("The shared grid's previous readings are addressed by market", t, func() {
		client := store.Vector_ServerToClient(store.NewVector())
		defer client.Release()
		for index, symbol := range []string{"BTC/USD", "ETH/USD"} {
			So(writeVectorMarket(client, symbol, []float64{float64(index + 1)}), ShouldBeNil)
		}
		snapshot, release := client.Snapshot(context.Background(), nil)
		defer release()
		saved, err := snapshot.Struct()
		So(err, ShouldBeNil)
		encoded, err := saved.Data()
		So(err, ShouldBeNil)
		restored := store.Vector_ServerToClient(store.NewVector())
		defer restored.Release()
		future, release := restored.Restore(context.Background(), func(args runtime.Snapshot_restore_Params) error { return args.SetData(encoded) })
		_, err = future.Struct()
		release()
		So(err, ShouldBeNil)
		for _, owner := range []store.Vector{client, restored} {
			for index, symbol := range []string{"BTC/USD", "ETH/USD", "BTC/USD"} {
				So(writeVectorMarket(owner, symbol, nil), ShouldBeNil)
				future, release := owner.Done(context.Background(), nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				values, err := result.Values()
				So(err, ShouldBeNil)
				So(values.Len(), ShouldEqual, 1)
				So(values.At(0), ShouldEqual, index%2+1)
				release()
			}
		}
	})
}

/* writeVectorMarket selects one series and optionally replaces its reading. */
func writeVectorMarket(client store.Vector, symbol string, values []float64) error {
	err := client.Write(context.Background(), func(args store.Vector_write_Params) error {
		args.SetWidth(1)
		scopes, err := args.NewScope(1)
		if err != nil {
			return err
		}
		if err := scopes.Set(0, symbol); err != nil {
			return err
		}
		if values == nil {
			return nil
		}
		indices, err := args.NewIndex(int32(len(values)))
		if err != nil {
			return err
		}
		records, err := args.NewValues(int32(len(values)))
		if err != nil {
			return err
		}
		for index, value := range values {
			indices.Set(index, int64(index))
			records.Set(index, value)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return client.WaitStreaming()
}

/* BenchmarkVectorWriteMarkets retains the shipping 411-coordinate previous cut per market. */
func BenchmarkVectorWriteMarkets(b *testing.B) {
	client := store.Vector_ServerToClient(store.NewVector())
	defer client.Release()
	values := make([]float64, 411)
	for index := range values {
		values[index] = float64(index)
	}
	markets := []string{"BTC/USD", "ETH/USD"}
	for _, symbol := range markets {
		if err := writeVectorMarket(client, symbol, values); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := writeVectorMarket(client, markets[index%len(markets)], values); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
