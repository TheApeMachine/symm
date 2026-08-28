package types

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
projectedMetrics loads signal/metric_map.csv — the canonical machine-readable
audit of every (source, metric) identity the production signals project — and
returns it as a source → metric set. It is the conformance oracle for
CategorySchemas: a schema row must reference a metric that is actually
projected by the declared source, so a typoed or nonexistent identity fails
the test rather than silently voting never.
*/
func projectedMetrics(t *testing.T) map[string]map[string]bool {
	t.Helper()

	path := filepath.Join("..", "signal", "metric_map.csv")

	file, err := os.Open(path)
	So(err, ShouldBeNil)

	if file == nil {
		return map[string]map[string]bool{}
	}

	defer file.Close()

	reader := csv.NewReader(file)
	projected := map[string]map[string]bool{}
	readHeader := true

	for {
		record, readErr := reader.Read()

		if readErr == io.EOF {
			break
		}

		So(readErr, ShouldBeNil)

		if readHeader {
			readHeader = false

			continue
		}

		if len(record) < 3 {
			continue
		}

		source, metric := record[0], record[1]

		if source == "" || metric == "" {
			continue
		}

		if projected[source] == nil {
			projected[source] = map[string]bool{}
		}

		projected[source][metric] = true
	}

	return projected
}

/*
hasSchemaLeg reports whether a (source, metric) evidence leg is declared in
CategorySchemas.
*/
func hasSchemaLeg(source SourceType, metric string) bool {
	for _, schema := range CategorySchemas {
		if schema.Source == source && schema.Metric == metric {
			return true
		}
	}

	return false
}

func TestCategorySchemas(t *testing.T) {
	Convey("Given the projected metric audit", t, func() {
		projected := projectedMetrics(t)
		So(projected, ShouldNotBeEmpty)

		Convey("every CategorySchema references a metric projected by its source", func() {
			for _, schema := range CategorySchemas {
				So(projected[string(schema.Source)], ShouldContainKey, schema.Metric)
			}
		})

		Convey("no schema references the typoed exhaust identity", func() {
			So(hasSchemaLeg("exhaustion", "depth_ask_divergence_velocity"), ShouldBeFalse)
		})

		Convey("semantically invalid single-metric-to-label leaps are absent", func() {
			// search-domain provenance, not relationship strength
			So(hasSchemaLeg(SourceLeadLag, "lag_fraction"), ShouldBeFalse)
			// one touch imbalance cannot justify spoof evidence
			So(hasSchemaLeg(SourceDepthFlow, "touch_imbalance"), ShouldBeFalse)
			// same raw metric cannot serve both scarcity and robustness without a polarity/transform
			So(hasSchemaLeg(SourceLiquidity, "touch_notional_imbalance"), ShouldBeFalse)
			// positive breadth cannot feed a slump without an explicit polarity transform
			So(hasSchemaLeg(SourceSentiment, "advance_fraction"), ShouldBeFalse)
			// side imbalance alone does not establish a squeeze
			So(hasSchemaLeg(SourceDerivatives, "liquidation_signed_fraction"), ShouldBeFalse)
		})

		Convey("no exhaust evidence leg remains", func() {
			for _, schema := range CategorySchemas {
				So(schema.Source, ShouldNotEqual, SourceType("exhaustion"))
			}
		})

		Convey("withdrawal unusualness no longer feeds a bluff label alone", func() {
			So(hasSchemaLeg(SourceToxicity, "withdrawal_fraction_zscore:bid"), ShouldBeFalse)
			So(hasSchemaLeg(SourceToxicity, "withdrawal_fraction_zscore:ask"), ShouldBeFalse)
		})
	})
}

func TestCategorySchemasNoFakeReplacement(t *testing.T) {
	Convey("Given the cleaned schema", t, func() {
		Convey("categories flagged as single-metric leaps are no longer produced by any schema row", func() {
			orphaned := map[CategoryType]bool{
				SpoofTrap:    false,
				ShortSqueeze: false,
				ToxicBluff:   false,
			}

			for _, schema := range CategorySchemas {
				if _, tracked := orphaned[schema.Category]; tracked {
					orphaned[schema.Category] = true
				}
			}

			So(orphaned[SpoofTrap], ShouldBeFalse)
			So(orphaned[ShortSqueeze], ShouldBeFalse)
			So(orphaned[ToxicBluff], ShouldBeFalse)
		})
	})
}
