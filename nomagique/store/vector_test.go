package store_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
