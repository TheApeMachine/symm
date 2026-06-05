package liquidity

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

func withLiquidityRawDump(t *testing.T, path string) {
	t.Helper()

	viper.Set("signals.liquidity.raw_dump", true)
	viper.Set("signals.liquidity.raw_dump_file", path)

	t.Cleanup(func() {
		viper.Set("signals.liquidity.raw_dump", false)
		viper.Set("signals.liquidity.raw_dump_file", "")
	})
}

func TestRawJSONLWriterWrite_Liquidity(t *testing.T) {
	Convey("Given a liquidity raw JSONL writer targeting a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "liquidity_raw.jsonl")
		withLiquidityRawDump(t, path)

		writer := rawdump.Open("liquidity")
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

func TestObserveRawJSONL_Liquidity(t *testing.T) {
	Convey("Given a liquidity signal writing raw JSONL to a temp file", t, func() {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "liquidity_raw.jsonl")
		withLiquidityRawDump(t, path)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		signal.symbols.Store("ALT/EUR", 1250.0)
		signal.symbols.Store("COIN/EUR", 800.0)
		signal.symbols.Store("PEER/EUR", 900.0)

		Convey("When publishTickers processes a ticker", func() {
			err := signal.publishTickers([]market.TickerUpdate{{
				Symbol: "ALT/EUR",
				Last:   10,
				Volume: 125,
			}})
			So(err, ShouldBeNil)

			err = signal.Close()
			So(err, ShouldBeNil)

			Convey("It should append the raw values", func() {
				records, err := readRawJSONLRecords(path)
				So(err, ShouldBeNil)
				So(records, ShouldHaveLength, 1)
				So(records[0].Symbol, ShouldEqual, "ALT/EUR")
				So(records[0].TimestampUnixNano, ShouldBeGreaterThan, 0)
			})
		})
	})
}
