package pumpdump

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/rawdump"
)

func readRawJSONLRecords(path string) ([]rawRecord, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	records := make([]rawRecord, 0)

	for scanner.Scan() {
		var record rawRecord

		if err := sonic.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, scanner.Err()
}

func withPumpdumpRawDump(t *testing.T, path string) {
	t.Helper()

	viper.Set("signals.pumpdump.raw_dump", true)
	viper.Set("signals.pumpdump.raw_dump_file", path)

	t.Cleanup(func() {
		viper.Set("signals.pumpdump.raw_dump", false)
		viper.Set("signals.pumpdump.raw_dump_file", "")
	})
}

func TestRawJSONLWriterWrite(t *testing.T) {
	Convey("Given a raw JSONL writer targeting a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "pumpdump_raw.jsonl")
		withPumpdumpRawDump(t, path)

		writer := rawdump.Open("pumpdump")
		timestamp := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

		Convey("When pre-classification readings are written", func() {
			err := writer.Write(rawRecord{
				TimestampUnixNano: timestamp.UnixNano(),
				Symbol:            "ALT/EUR",
				Price:             10.5,
				Qty:               1.25,
				Side:              "buy",
				Anchor:            10.0,
				VolumeSum:         12.5,
				RVOL:              1.8,
				Precursor:         0.05,
			})
			So(err, ShouldBeNil)

			err = writer.Close()
			So(err, ShouldBeNil)

			Convey("It should append one JSON object per line", func() {
				records, err := readRawJSONLRecords(path)
				So(err, ShouldBeNil)
				So(records, ShouldHaveLength, 1)
				So(records[0].TimestampUnixNano, ShouldEqual, timestamp.UnixNano())
				So(records[0].Symbol, ShouldEqual, "ALT/EUR")
				So(records[0].Side, ShouldEqual, "buy")
				So(records[0].RVOL, ShouldEqual, 1.8)
				So(records[0].Precursor, ShouldEqual, 0.05)
			})
		})
	})
}

func TestObserveRawJSONL(t *testing.T) {
	Convey("Given a pumpdump signal writing raw JSONL to a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "pumpdump_raw.jsonl")
		withPumpdumpRawDump(t, path)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		Convey("When observe processes a trade", func() {
			err := signal.observe(market.TradeUpdate{
				Symbol:    "ALT/EUR",
				Side:      "buy",
				Price:     10,
				Qty:       1.5,
				Timestamp: base,
			})
			So(err, ShouldBeNil)

			err = signal.Close()
			So(err, ShouldBeNil)

			Convey("It should append the raw values before classification", func() {
				records, err := readRawJSONLRecords(path)
				So(err, ShouldBeNil)
				So(records, ShouldHaveLength, 1)
				So(records[0].TimestampUnixNano, ShouldEqual, base.UnixNano())
				So(records[0].Symbol, ShouldEqual, "ALT/EUR")
			})
		})
	})
}
