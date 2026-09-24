package temporal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	random "math/rand/v2"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
frames writes one session's frames, all from one endpoint, as a miner receives
them from a tape.
*/
func frames(params Mine_write_Params, first int64, payloads ...[]byte) error {
	for _, err := range []error{params.SetSession("test"), params.SetChannel("ticker"), params.SetPriceField("last")} {
		if err != nil {
			return err
		}
	}

	list, err := params.NewPayload(int32(len(payloads)))

	if err != nil {
		return err
	}

	sequences, err := params.NewSequence(int32(len(payloads)))

	if err != nil {
		return err
	}

	endpoints, err := params.NewEndpoint(int32(len(payloads)))

	if err != nil {
		return err
	}

	for index, payload := range payloads {
		sequences.Set(index, first+int64(index))

		if err := list.Set(index, payload); err != nil {
			return err
		}

		if err := endpoints.Set(index, "fixture"); err != nil {
			return err
		}
	}

	return nil
}

func TestMineWrite(t *testing.T) {
	Convey("Given a miner receiving a malformed multi-symbol frame", t, func() {
		server := NewMine()
		client := Mine_ServerToClient(server)
		defer client.Release()
		So(client.Write(context.Background(), func(params Mine_write_Params) error {
			return frames(params, 0, []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":100},{"symbol":"ETH/USD"}]}`))
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
		So(server.paths, ShouldBeEmpty)
		So(server.sequences, ShouldBeEmpty)
	})

	Convey("Given a ticker frame where one pair has never traded", t, func() {
		server := NewMine()
		client := Mine_ServerToClient(server)
		defer client.Release()
		So(client.Write(context.Background(), func(params Mine_write_Params) error {
			// Kraken reports last 0 for a pair with no trades (CORN/USD, captured 2026-09-23).
			return frames(params, 0, []byte(`{"channel":"ticker","data":[{"symbol":"CORN/USD","last":0,"trades":0},{"symbol":"BTC/USD","last":100}]}`))
		}), ShouldBeNil)

		Convey("Then the unpriced pair is absent and the priced one keeps its record position", func() {
			So(client.WaitStreaming(), ShouldBeNil)
			So(server.paths, ShouldHaveLength, 1)

			for key, path := range server.paths {
				So(key.symbol, ShouldEqual, "BTC/USD")
				So(path.cursors, ShouldResemble, []TapeCursor{{Sequence: 0, Record: 1}})
			}
		})
	})

	Convey("Given a rise and its fall handed over together", t, func() {
		client := Mine_ServerToClient(NewMine())
		defer client.Release()
		payloads := make([][]byte, 0, 180)

		for index := 0; index < 180; index++ {
			exponent := float64(index) * 0.005

			if index >= 60 {
				exponent = 59*0.005 - float64(index-59)*0.008
			}

			payload, err := json.Marshal(map[string]any{"channel": "ticker", "data": []map[string]any{{"symbol": "BTC/USD", "last": 100 * math.Exp(exponent)}}})
			So(err, ShouldBeNil)
			payloads = append(payloads, payload)
		}

		So(client.Write(context.Background(), func(params Mine_write_Params) error {
			return frames(params, 0, payloads...)
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)

		future, release := client.Done(context.Background(), nil)
		defer release()
		results, err := future.Struct()
		So(err, ShouldBeNil)

		Convey("Then each frame was mined in order, and the confirmed move is archived once and batched per stream", func() {
			So(results.Which(), ShouldEqual, Mined_Which_events)
			rows, err := results.Events().Out()
			So(err, ShouldBeNil)
			batches, err := results.Events().Batch()
			So(err, ShouldBeNil)
			So(rows.Len(), ShouldBeGreaterThan, 0)
			So(batches.Len(), ShouldEqual, 1)

			raw, err := batches.At(0)
			So(err, ShouldBeNil)
			var batch MiningBatch
			So(json.Unmarshal(raw, &batch), ShouldBeNil)
			So(batch.Session, ShouldEqual, "test")
			So(batch.Endpoint, ShouldEqual, "fixture")
			So(batch.Events, ShouldNotBeEmpty)
			So(batch.Events[0].B.Sequence, ShouldBeLessThan, batch.Events[0].C.Sequence)

			Convey("And the next evaluation starts clean", func() {
				So(client.Write(context.Background(), func(params Mine_write_Params) error {
					return frames(params, 180)
				}), ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)
				next, release := client.Done(context.Background(), nil)
				defer release()
				quiet, err := next.Struct()
				So(err, ShouldBeNil)
				So(quiet.Which(), ShouldEqual, Mined_Which_none)
			})
		})
	})
}

func TestMinedPathEvent(t *testing.T) {
	Convey("Given a confirmed move after a precursor", t, func() {
		path := &minedPath{excursion: NewExcursion(context.Background())}
		// Three sustained legs exercise both directions and the separate confirmation cursor.
		for index := 0; index < 180; index++ {
			phase := index / 60
			offset := index % 60
			exponent := float64(offset) * 0.005

			if phase == 1 {
				exponent = 59*0.005 - float64(offset)*0.008
			}
			if phase == 2 {
				exponent = 59*0.005 - 59*0.008 + float64(offset)*0.008
			}
			path.cursors = append(path.cursors, TapeCursor{Sequence: int64(index), Record: 1})
			So(path.excursion.Step(100*math.Exp(exponent)), ShouldBeNil)
		}
		event, err := path.event("BTC/USD", path.cursors[len(path.cursors)-1])
		So(err, ShouldBeNil)
		So(event.A, ShouldNotBeNil)
		So(event.A.Sequence, ShouldBeGreaterThanOrEqualTo, event.Precursor.Sequence)
		So(event.A.Sequence, ShouldBeLessThan, event.B.Sequence)
		So(event.C.Sequence, ShouldBeGreaterThan, event.B.Sequence)
		So(event.D.Sequence, ShouldBeGreaterThan, event.C.Sequence)
		decoded, err := hex.DecodeString(event.Seed)
		So(err, ShouldBeNil)
		var seed [32]byte
		copy(seed[:], decoded)
		move := path.excursion.reported
		index := random.New(random.NewChaCha8(seed)).IntN(move.ignitionIndex-move.anchorIndex) + move.anchorIndex
		So(*event.A, ShouldResemble, path.cursors[index])
	})
}

func BenchmarkMineWrite(b *testing.B) {
	client := Mine_ServerToClient(NewMine())
	defer client.Release()
	ctx := context.Background()
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		// Alternating long legs generate confirmed events as well as quiet observations.
		offset := index % 120

		if offset > 60 {
			offset = 120 - offset
		}
		payload, err := json.Marshal(map[string]any{"channel": "ticker", "data": []map[string]any{{"symbol": "BTC/USD", "last": 100 * math.Exp(float64(offset)*0.005)}, {"symbol": "ETH/USD", "last": 200}}})

		if err != nil {
			b.Fatal(err)
		}
		err = client.Write(ctx, func(params Mine_write_Params) error {
			return frames(params, int64(index), payload)
		})

		if err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err = future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}
