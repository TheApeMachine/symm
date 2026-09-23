package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestTallyWrite(t *testing.T) {
	Convey("Given explicit categorical counts", t, func() {
		for _, example := range []struct {
			counts, category, expected string
			query                      bool
			total                      uint64
		}{
			{`{}`, "", `{}`, true, 0},
			{`{}`, "WAIT", `{"WAIT":1}`, false, 1},
			{`{"WAIT":2}`, "ENTER", `{"ENTER":1,"WAIT":2}`, false, 3},
			{`{"WAIT":9007199254740993}`, "WAIT", `{"WAIT":9007199254740994}`, false, 9007199254740994},
			{`{"WAIT":2}`, "EXIT", `{"WAIT":2}`, true, 2},
		} {
			client := statistic.Tally_ServerToClient(statistic.NewTally())
			defer client.Release()
			So(client.Write(context.Background(), func(params statistic.Tally_write_Params) error {
				params.SetQuery(example.query)

				if err := params.SetCounts([]byte(example.counts)); err != nil {
					return err
				}

				return params.SetCategory(example.category)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			counts, err := result.Counts()
			So(err, ShouldBeNil)
			So(string(counts), ShouldEqual, example.expected)
			So(result.Total(), ShouldEqual, example.total)
			release()
		}

		Convey("Then absent, invalid and overflowing counts are rejected", func() {
			for _, payload := range []string{"", `null`, `[]`, `{"A":null}`, `{"A":0}`, `{"A":-1}`, `{"A":1.5}`, `{"A":18446744073709551615,"B":1}`} {
				client := statistic.Tally_ServerToClient(statistic.NewTally())
				err := client.Write(context.Background(), func(params statistic.Tally_write_Params) error {
					params.SetQuery(true)
					return params.SetCounts([]byte(payload))
				})

				if err == nil {
					err = client.WaitStreaming()
				}

				So(err, ShouldNotBeNil)
				client.Release()
			}
		})
	})
}

func BenchmarkTallyWrite(b *testing.B) {
	client := statistic.Tally_ServerToClient(statistic.NewTally())
	defer client.Release()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := client.Write(context.Background(), func(params statistic.Tally_write_Params) error {
			if err := params.SetCounts([]byte(`{"WAIT":500,"ENTER":40,"EXIT":37}`)); err != nil {
				return err
			}

			return params.SetCategory("ENTER")
		}); err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
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
