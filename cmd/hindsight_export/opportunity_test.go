package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gocloud.dev/blob/memblob"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
)

func TestWriteOpportunityRecords(t *testing.T) {
	Convey("Given a captured ticker tape with a completed upward leg", t, func() {
		engine := memblob.OpenBucket(nil)
		Reset(func() { So(engine.Close(), ShouldBeNil) })

		runID := "run-opportunities"
		So(store.Write(context.Background(), engine, hindsight.RunID("run-opportunities").Prefix("runs")+"run.json", hindsight.Run{
			ID:        hindsight.RunID(runID),
			StartedAt: time.Unix(1, 0),
		}), ShouldBeNil)

		for index := 1; index <= 80; index++ {
			price := 100.0

			if index > 64 && index <= 76 {
				price += float64(index-64) * 0.5
			}

			if index > 76 {
				price = 104
			}

			payload := []byte(fmt.Sprintf(
				`{"channel":"ticker","type":"update","data":[{"symbol":"TEST/USD","bid":%.4f,"ask":%.4f}]}`,
				price-0.01,
				price+0.01,
			))
			identity := hindsight.CaptureIdentity{
				Run:            hindsight.RunID(runID),
				Sequence:       hindsight.CaptureSequence(index),
				Stream:         "spot:ticker",
				StreamEpoch:    1,
				StreamSequence: uint64(index),
			}
			So(store.Write(context.Background(), engine, identity.Key(), hindsight.RawFrame{
				Identity: identity, Endpoint: "wss://ws.kraken.com/v2", Kind: "ticker", Payload: payload, ReceivedAt: time.Unix(int64(index), 0),
			}), ShouldBeNil)
		}

		var output bytes.Buffer
		written, err := writeOpportunityRecords(
			engine,
			runID,
			json.NewEncoder(&output),
		)

		So(err, ShouldBeNil)
		So(written, ShouldBeGreaterThan, 0)

		var record opportunityRecord
		So(json.NewDecoder(&output).Decode(&record), ShouldBeNil)
		So(record.Kind, ShouldEqual, "episode")
		So(record.Episode.Kind, ShouldEqual, hindsight.EpisodeUpwardExcursion)
		So(record.HasExtremumBid, ShouldBeTrue)
		So(record.ExtremumBid, ShouldBeGreaterThan, 100.0)
	})
}
