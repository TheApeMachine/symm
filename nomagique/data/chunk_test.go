package data_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestChunk(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Chunk capability configured with size 200", t, func() {
		client := data.Chunk_ServerToClient(data.NewChunk())
		defer client.Release()

		chunk := func(input []string, size int64) ([][]string, error) {
			inputBytes, err := json.Marshal(input)
			So(err, ShouldBeNil)

			err = client.Write(ctx, func(params data.Chunk_write_Params) error {
				if err := params.SetData(inputBytes); err != nil {
					return err
				}
				params.SetSize(size)
				return nil
			})
			if err != nil {
				return nil, err
			}

			err = client.WaitStreaming()
			if err != nil {
				return nil, err
			}

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			if err != nil {
				return nil, err
			}

			var chunks [][]string

			extract := func(getter func() ([]byte, error)) {
				bytes, err := getter()
				if err == nil && len(bytes) > 0 {
					var parsed []string
					if json.Unmarshal(bytes, &parsed) == nil && len(parsed) > 0 {
						chunks = append(chunks, parsed)
					}
				}
			}

			extract(results.Out0)
			extract(results.Out1)
			extract(results.Out2)
			extract(results.Out3)

			ep0, _ := results.Endpoint0()
			ep1, _ := results.Endpoint1()
			ep2, _ := results.Endpoint2()
			ep3, _ := results.Endpoint3()

			endpoints := []string{}
			if ep0 != "" {
				endpoints = append(endpoints, ep0)
			}
			if ep1 != "" {
				endpoints = append(endpoints, ep1)
			}
			if ep2 != "" {
				endpoints = append(endpoints, ep2)
			}
			if ep3 != "" {
				endpoints = append(endpoints, ep3)
			}
			So(len(endpoints), ShouldEqual, len(chunks))

			return chunks, nil
		}

		Convey("A universe of 451 symbols splits into shards of 200, 200, and 51", func() {
			symbols := make([]string, 451)
			for index := range 451 {
				symbols[index] = fmt.Sprintf("SYM%d/USD", index)
			}

			shards, err := chunk(symbols, 200)
			So(err, ShouldBeNil)
			So(len(shards), ShouldEqual, 3)
			So(len(shards[0]), ShouldEqual, 200)
			So(len(shards[1]), ShouldEqual, 200)
			So(len(shards[2]), ShouldEqual, 51)
			So(shards[0][0], ShouldEqual, "SYM0/USD")
			So(shards[0][199], ShouldEqual, "SYM199/USD")
			So(shards[1][0], ShouldEqual, "SYM200/USD")
			So(shards[1][199], ShouldEqual, "SYM399/USD")
			So(shards[2][0], ShouldEqual, "SYM400/USD")
			So(shards[2][50], ShouldEqual, "SYM450/USD")
		})

		Convey("A small universe of 3 symbols stays in shard 0 alone", func() {
			symbols := []string{"BTC/USD", "RATE/USD", "ETH/USD"}

			shards, err := chunk(symbols, 200)
			So(err, ShouldBeNil)
			So(len(shards), ShouldEqual, 1)
			So(shards[0], ShouldResemble, symbols)
		})
	})
}
