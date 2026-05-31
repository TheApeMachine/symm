package replay

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestPerturbLineTimestampJitter(t *testing.T) {
	convey.Convey("Given timestamp jitter config", t, func() {
		line := Line{
			Timestamp: time.Unix(1_700_000_000, 0).UTC(),
			Transport: TransportWS,
			Channel:   "trade",
			Payload:   json.RawMessage(`{"channel":"trade","type":"update","data":[]}`),
		}
		config := PerturbConfigFrom(true, 42, 0, 50*time.Millisecond)
		random := NewPerturbRandom(config.Seed)

		perturbed, err := PerturbLine(line, config, random)

		convey.Convey("It should shift arrival timestamps within the jitter window", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(perturbed.Timestamp, convey.ShouldNotEqual, line.Timestamp)
			delta := perturbed.Timestamp.Sub(line.Timestamp)

			if delta < 0 {
				delta = -delta
			}

			convey.So(delta, convey.ShouldBeLessThanOrEqualTo, 50*time.Millisecond)
		})
	})
}

func TestPerturbBookPayloadQtyJitter(t *testing.T) {
	convey.Convey("Given a book delta payload", t, func() {
		payload := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/EUR","bids":[{"price":"50000","qty":"1.0"}],"asks":[{"price":"50001","qty":"2.0"}],"checksum":123,"timestamp":"2026-05-23T12:00:00.000000Z"}]}`)
		random := rand.New(rand.NewSource(7))

		perturbed, err := perturbBookPayload(payload, random, 0.05)

		convey.Convey("It should jitter quantities and clear checksum", func() {
			convey.So(err, convey.ShouldBeNil)

			var envelope socketEnvelope

			convey.So(json.Unmarshal(perturbed, &envelope), convey.ShouldBeNil)

			rows, decodeErr := decodeWireBookRows(envelope.Data)

			convey.So(decodeErr, convey.ShouldBeNil)
			convey.So(rows[0].Checksum, convey.ShouldEqual, 0)

			bidQty, qtyErr := rows[0].Bids[0].Qty.Float64()

			convey.So(qtyErr, convey.ShouldBeNil)
			convey.So(bidQty, convey.ShouldNotEqual, 1.0)
			convey.So(rows[0].Bids[0].Price.String(), convey.ShouldEqual, "50000")
		})
	})
}

func BenchmarkPerturbLine(b *testing.B) {
	line := Line{
		Timestamp: time.Now().UTC(),
		Transport: TransportWS,
		Channel:   bookChannel,
		Payload: json.RawMessage(
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/EUR","bids":[{"price":"50000","qty":"1.0"}],"asks":[{"price":"50001","qty":"2.0"}],"checksum":123,"timestamp":"2026-05-23T12:00:00.000000Z"}]}`,
		),
	}
	config := PerturbConfigFrom(true, 99, 0.05, 50*time.Millisecond)
	random := NewPerturbRandom(config.Seed)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = PerturbLine(line, config, random)
	}
}
