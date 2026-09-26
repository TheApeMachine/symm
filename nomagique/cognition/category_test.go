package cognition

import (
	"context"
	"math"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* categoryFixture presents causal standardized evidence through the native protocol. */
func categoryFixture(params Category_write_Params, values []float64, order []int, complete bool) error {
	record, err := params.NewCut()
	if err != nil {
		return err
	}
	record.SetTypeId(data.MetricCut_TypeID)
	cut, err := data.NewMetricCut(params.Segment())
	if err != nil {
		return err
	}
	cut.SetEpoch(9007199254740993)
	cut.SetSequence(11)
	cut.SetComplete(complete)
	if err := cut.SetSymbol("BTC/USD"); err != nil {
		return err
	}
	metrics, err := cut.NewMetrics(int32(len(order)))
	if err != nil {
		return err
	}
	names := []string{"trade:flow", "book:imbalance", "ticker:return"}
	for index, position := range order {
		metric := metrics.At(index)
		if err := metric.SetIdentity(names[position]); err != nil {
			return err
		}
		metric.SetValue(values[position])
		metric.SetPresent(true)
		metric.SetEpoch(cut.Epoch())
		metric.SetSequence(int64(position + 1))
	}
	if err := record.SetValue(capnp.Struct(cut).ToPtr()); err != nil {
		return err
	}
	identities, err := params.NewIdentities(4)
	if err != nil {
		return err
	}
	assignments, err := params.NewCategories(4)
	if err != nil {
		return err
	}
	for index, identity := range []string{names[0], names[1], names[2], "unavailable:coordinate"} {
		if err := identities.Set(index, identity); err != nil {
			return err
		}
	}
	for index, category := range []string{"flow", "flow", "trend", "trend"} {
		if err := assignments.Set(index, category); err != nil {
			return err
		}
	}
	vocabulary, err := params.NewVocabulary(2)
	if err != nil {
		return err
	}
	if err := vocabulary.Set(0, "flow"); err != nil {
		return err
	}
	return vocabulary.Set(1, "trend")
}

func TestCategoryWrite(t *testing.T) {
	Convey("One native cut is one current category verdict regardless of arrival order", t, func() {
		client := Category_ServerToClient(NewCategory())
		defer client.Release()
		ctx := context.Background()
		for _, regime := range []struct {
			values      []float64
			expected    string
			probability float64
		}{
			{[]float64{9, 4, 1}, "flow", 7.0 / 9},
			{[]float64{0, 0, 8}, "trend", 9.0 / 10},
			{[]float64{-9, -4, 1}, "flow", 7.0 / 9},
			{[]float64{0, 0, 0}, "flow", 0.5},
		} {
			for _, order := range [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}} {
				So(client.Write(ctx, func(params Category_write_Params) error { return categoryFixture(params, regime.values, order, true) }), ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, CategoryResult_Which_ready)
				reading, err := result.Ready().Reading()
				So(err, ShouldBeNil)
				dominant, err := reading.Dominant()
				So(err, ShouldBeNil)
				So(dominant, ShouldEqual, regime.expected)
				So(reading.Epoch(), ShouldEqual, int64(9007199254740993))
				categories, err := reading.Categories()
				So(err, ShouldBeNil)
				sum := 0.0
				for index := range categories.Len() {
					category := categories.At(index)
					name, err := category.Name()
					So(err, ShouldBeNil)
					sum += category.Confidence()
					So(category.Surprisal(), ShouldAlmostEqual, -math.Log2(category.Confidence()))
					if name == regime.expected {
						So(category.Confidence(), ShouldAlmostEqual, regime.probability)
					}
				}
				So(sum, ShouldAlmostEqual, 1)
				missing, err := categories.At(1).Missing()
				So(err, ShouldBeNil)
				So(missing.Len(), ShouldEqual, 1)
				release()
			}
		}
		Convey("Incomplete cuts cannot classify or inherit the preceding verdict", func() {
			So(client.Write(ctx, func(params Category_write_Params) error {
				return categoryFixture(params, []float64{9, 4, 1}, []int{0, 1, 2}, false)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, CategoryResult_Which_idle)
		})
	})
	Convey("Evidence beyond the causal watermark is rejected", t, func() {
		client := Category_ServerToClient(NewCategory())
		defer client.Release()
		err := client.Write(context.Background(), func(params Category_write_Params) error {
			if err := categoryFixture(params, []float64{1, 2, 3}, []int{0, 1, 2}, true); err != nil {
				return err
			}
			record, err := params.Cut()
			if err != nil {
				return err
			}
			pointer, err := record.Value()
			if err != nil {
				return err
			}
			metrics, err := data.MetricCut(pointer.Struct()).Metrics()
			if err != nil {
				return err
			}
			metrics.At(0).SetSequence(12)
			return nil
		})
		So(err, ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
	})
}

func BenchmarkCategoryWrite(b *testing.B) {
	client := Category_ServerToClient(NewCategory())
	defer client.Release()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(params Category_write_Params) error {
			return categoryFixture(params, []float64{9, 4, 1}, []int{2, 0, 1}, true)
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
