package hawkes

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

func withHawkesRawDump(t *testing.T, path string) {
	t.Helper()

	viper.Set("signals.hawkes.raw_dump", true)
	viper.Set("signals.hawkes.raw_dump_file", path)

	t.Cleanup(func() {
		viper.Set("signals.hawkes.raw_dump", false)
		viper.Set("signals.hawkes.raw_dump_file", "")
	})
}

func TestRawJSONLWriterWrite_Hawkes(t *testing.T) {
	Convey("Given a hawkes raw JSONL writer targeting a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "hawkes_raw.jsonl")
		withHawkesRawDump(t, path)

		writer := rawdump.Open("hawkes")
		timestamp := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

		Convey("When a classified reading is written", func() {
			err := writer.Write(rawRecord{
				TimestampUnixNano: timestamp.UnixNano(),
				Symbol:            "ALT/EUR",
				Last:              10.5,
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
			})
		})
	})
}

func TestObserveRawJSONL_Hawkes(t *testing.T) {
	Convey("Given a hawkes signal writing raw JSONL to a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "hawkes_raw.jsonl")
		withHawkesRawDump(t, path)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		trades := tradeBurst("ALT/EUR", base, 128)

		Convey("When observeTrades processes a burst", func() {
			err := signal.observeTrades(trades)
			So(err, ShouldBeNil)

			err = signal.Close()
			So(err, ShouldBeNil)

			Convey("It should append raw values for the symbol", func() {
				records, err := readRawJSONLRecords(path)
				So(err, ShouldBeNil)
				So(records, ShouldNotBeEmpty)
				So(records[0].Symbol, ShouldEqual, "ALT/EUR")
				So(records[0].TimestampUnixNano, ShouldBeGreaterThan, 0)
			})
		})
	})
}
