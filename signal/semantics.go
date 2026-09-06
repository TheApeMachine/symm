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
	"embed"
	"io/fs"
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
	Definedness  string `json:"definedness,omitempty"`
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
		QualityDefinedness    string `json:"quality_definedness"`
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
	Signals        map[string]SignalSemantics `json:"signals"`
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
		metricMapLoaded.Signals = SignalPurposes()

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
				Definedness:  strings.TrimSpace(entry.QualityDefinedness),
				Destinations: strings.TrimSpace(entry.NormativeDestinations),
				Forbidden:    strings.TrimSpace(entry.ForbiddenUse),
				Status:       entry.NormativeStatus,
			}
		}
	})

	return metricMapLoaded
}

//go:embed */README.md
var signalSpecSource embed.FS

/*
SignalSemantics is one signal family's declared identity: the Purpose statement
its own specification opens with. It answers "what does this whole family of
numbers measure?" for a reader who does not already know the family.
*/
type SignalSemantics struct {
	Source  string `json:"source"`
	Purpose string `json:"purpose"`
}

var (
	signalMapOnce   sync.Once
	signalMapLoaded map[string]SignalSemantics
)

/*
SignalPurposes returns each signal family's declared purpose, read from the
family's own README. The statement is quoted, never paraphrased: a family with
no specification is reported as having none rather than being described.

Only the leading prose of the Purpose section is taken. What follows it is the
enumerated measurement list, which belongs to the specification rather than to
a one-line statement of what the family is.
*/
func SignalPurposes() map[string]SignalSemantics {
	signalMapOnce.Do(func() {
		signalMapLoaded = make(map[string]SignalSemantics)

		entries, err := fs.Glob(signalSpecSource, "*/README.md")

		if err != nil {
			panic("signal: glob embedded specifications: " + err.Error())
		}

		for _, path := range entries {
			body, err := signalSpecSource.ReadFile(path)

			if err != nil {
				panic("signal: read embedded specification " + path + ": " + err.Error())
			}

			source := strings.TrimSuffix(path, "/README.md")
			purpose := declaredPurpose(string(body))

			if purpose == "" {
				continue
			}

			signalMapLoaded[source] = SignalSemantics{
				Source:  source,
				Purpose: purpose,
			}
		}
	})

	return signalMapLoaded
}

/*
declaredPurpose lifts the leading prose of a specification's Purpose section.

It stops at the next heading, and at the line where the specification turns
from stating what the family is into enumerating what it measures — that list
is the specification's own content, not a statement of identity.
*/
func declaredPurpose(specification string) string {
	lines := strings.Split(specification, "\n")
	statement := make([]string, 0, 4)
	inside := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			if inside {
				break
			}

			inside = purposeHeading(trimmed)

			continue
		}

		if !inside || trimmed == "" {
			continue
		}

		if strings.Trim(trimmed, "-") == "" {
			break
		}

		if enumerationLead(trimmed) {
			break
		}

		statement = append(statement, trimmed)
	}

	return strings.Join(statement, " ")
}

/* purposeHeading reports whether a heading opens the Purpose section. */
func purposeHeading(heading string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(heading)), "purpose")
}

/*
enumerationLead reports whether a line hands off from the identity statement to
the specification's enumerated measurement list.
*/
func enumerationLead(line string) bool {
	lowered := strings.ToLower(line)

	for _, lead := range []string{"it measures", "it answers", "for aggressive"} {
		if strings.HasPrefix(lowered, lead) {
			return true
		}
	}

	return false
}
