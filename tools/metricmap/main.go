/* Command metricmap builds signal/metric_map.json from its CSV authority. */
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type metricEntry struct {
	Source                string `json:"source"`
	Metric                string `json:"metric"`
	Identity              string `json:"identity"`
	MetricClass           string `json:"metric_class"`
	SemanticRole          string `json:"semantic_role"`
	Purpose               string `json:"purpose"`
	QualityDefinedness    string `json:"quality_definedness"`
	CurrentNamedUse       string `json:"current_named_use"`
	NormativeDestinations string `json:"normative_destinations"`
	ForbiddenUse          string `json:"forbidden_use"`
	ReviewNote            string `json:"review_note"`
	NormativeStatus       string `json:"normative_status"`
	BaselineCommit        string `json:"baseline_commit"`
}

type metricCatalog struct {
	BaselineCommit string        `json:"baseline_commit"`
	Metrics        []metricEntry `json:"metrics"`
}

func readCatalog(reader io.Reader) (metricCatalog, error) {
	records, err := csv.NewReader(reader).ReadAll()

	if err != nil {
		return metricCatalog{}, fmt.Errorf("metricmap: read csv: %w", err)
	}

	if len(records) < 2 {
		return metricCatalog{}, fmt.Errorf("metricmap: metric rows required")
	}

	headings := make(map[string]int, len(records[0]))

	for index, heading := range records[0] {
		headings[heading] = index
	}

	required := []string{
		"source", "metric", "identity", "metric_class", "semantic_role",
		"purpose", "quality_definedness", "current_named_use",
		"normative_destinations", "forbidden_use", "review_note",
		"normative_status", "baseline_commit",
	}

	for _, heading := range required {
		if _, found := headings[heading]; !found {
			return metricCatalog{}, fmt.Errorf("metricmap: required column %q missing", heading)
		}
	}

	catalog := metricCatalog{Metrics: make([]metricEntry, 0, len(records)-1)}
	seen := make(map[string]struct{}, len(records)-1)
	baseline := ""
	mixed := false

	for rowIndex, record := range records[1:] {
		value := func(name string) string { return record[headings[name]] }
		entry := metricEntry{
			Source: value("source"), Metric: value("metric"), Identity: value("identity"),
			MetricClass: value("metric_class"), SemanticRole: value("semantic_role"),
			Purpose: value("purpose"), QualityDefinedness: value("quality_definedness"),
			CurrentNamedUse:       value("current_named_use"),
			NormativeDestinations: value("normative_destinations"),
			ForbiddenUse:          value("forbidden_use"), ReviewNote: value("review_note"),
			NormativeStatus: value("normative_status"), BaselineCommit: value("baseline_commit"),
		}

		if entry.Identity != entry.Source+"/"+entry.Metric {
			return metricCatalog{}, fmt.Errorf("metricmap: row %d identity mismatch", rowIndex+2)
		}

		if _, found := seen[entry.Identity]; found {
			return metricCatalog{}, fmt.Errorf("metricmap: duplicate identity %q", entry.Identity)
		}

		seen[entry.Identity] = struct{}{}
		catalog.Metrics = append(catalog.Metrics, entry)

		if baseline == "" {
			baseline = entry.BaselineCommit
		}

		mixed = mixed || entry.BaselineCommit != baseline
	}

	catalog.BaselineCommit = baseline

	if mixed {
		catalog.BaselineCommit = "mixed"
	}

	return catalog, nil
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: metricmap <metric_map.csv> <metric_map.json>")
	}

	input, err := os.Open(os.Args[1])

	if err != nil {
		panic(err)
	}

	catalog, err := readCatalog(input)
	_ = input.Close()

	if err != nil {
		panic(err)
	}

	encoded, err := json.MarshalIndent(catalog, "", "  ")

	if err != nil {
		panic(err)
	}

	encoded = append(encoded, '\n')

	if err := os.WriteFile(os.Args[2], encoded, 0o644); err != nil {
		panic(err)
	}
}
