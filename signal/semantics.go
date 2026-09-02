/*
Package signal serves the declared semantic identity of every production
(source, metric) pair.

METRIC_MAP.md defines the semantic architecture. metric_map.csv is the current
catalog authority, and metric_map.json is its generated runtime form. The map
answers, for one metric: why the fact exists, what it may legitimately affect,
and — the part that matters most to an inspection surface — what must never be
inferred from it.

Inspection displays these statements verbatim. It never paraphrases a purpose
into a verdict, and never turns a forbidden use into a warning about a trade.
*/
package signal

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
)

//go:embed metric_map.json
var metricMapSource []byte

/*
MetricSemantics is one metric's declared identity: the projection of a
metric_map.json entry that an inspection view needs to say what a number
physically means.
*/
type MetricSemantics struct {
	Identity     string `json:"identity"`
	Source       string `json:"source"`
	Metric       string `json:"metric"`
	Class        string `json:"class,omitempty"`
	Role         string `json:"role,omitempty"`
	Purpose      string `json:"purpose,omitempty"`
	Destinations string `json:"destinations,omitempty"`
	Forbidden    string `json:"forbidden,omitempty"`
	Status       string `json:"status,omitempty"`
}

type metricMapFile struct {
	BaselineCommit string `json:"baseline_commit"`
	Metrics        []struct {
		Source                string `json:"source"`
		Metric                string `json:"metric"`
		Identity              string `json:"identity"`
		MetricClass           string `json:"metric_class"`
		SemanticRole          string `json:"semantic_role"`
		Purpose               string `json:"purpose"`
		NormativeDestinations string `json:"normative_destinations"`
		ForbiddenUse          string `json:"forbidden_use"`
		NormativeStatus       string `json:"normative_status"`
	} `json:"metrics"`
}

/*
MetricMap is the decoded map keyed by declared identity ("source/metric"),
alongside the commit the map was baselined against. The baseline travels with
the map because a semantic statement about a metric is only as current as the
code it was written against.
*/
type MetricMap struct {
	BaselineCommit string                     `json:"baselineCommit"`
	Metrics        map[string]MetricSemantics `json:"metrics"`
}

var (
	metricMapOnce   sync.Once
	metricMapLoaded MetricMap
)

/* Semantics returns the declared semantic map, decoded once. */
func Semantics() MetricMap {
	metricMapOnce.Do(func() {
		var file metricMapFile

		metricMapLoaded = MetricMap{Metrics: make(map[string]MetricSemantics)}

		if err := sonic.Unmarshal(metricMapSource, &file); err != nil {
			panic("signal: decode embedded metric map: " + err.Error())
		}

		metricMapLoaded.BaselineCommit = file.BaselineCommit

		for _, entry := range file.Metrics {
			identity := entry.Identity

			if identity == "" {
				identity = entry.Source + "/" + entry.Metric
			}

			metricMapLoaded.Metrics[identity] = MetricSemantics{
				Identity:     identity,
				Source:       entry.Source,
				Metric:       entry.Metric,
				Class:        entry.MetricClass,
				Role:         entry.SemanticRole,
				Purpose:      strings.TrimSpace(entry.Purpose),
				Destinations: strings.TrimSpace(entry.NormativeDestinations),
				Forbidden:    strings.TrimSpace(entry.ForbiddenUse),
				Status:       entry.NormativeStatus,
			}
		}
	})

	return metricMapLoaded
}

/*
Lookup returns the declared semantics of one (source, metric) pair. A metric
with no declared entry is reported as undeclared rather than given a plausible
description — an inspection surface may not invent the meaning of a number.
*/
func Lookup(source, metric string) (MetricSemantics, bool) {
	semantics, found := Semantics().Metrics[source+"/"+metric]

	return semantics, found
}
