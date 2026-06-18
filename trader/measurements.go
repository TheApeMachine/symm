package trader

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/resonance"
)

/*
collectMeasureScopes returns symbol scopes seen in ingest or measurement rows.
*/
func (crypto *Crypto) collectMeasureScopes() []string {
	scopes := make(map[string]struct{})

	for _, symbol := range viper.GetStringSlice("market.default_symbols") {
		trimmed := strings.TrimSpace(symbol)

		if trimmed != "" {
			scopes[trimmed] = struct{}{}
		}
	}

	for _, prefix := range []string{"measurement/", "ticker/", "book/", "trade/"} {
		for inbound := range crypto.tree.Seek([]byte(prefix)) {
			scope, _ := inbound.Scope()

			if scope != "" {
				scopes[scope] = struct{}{}
			}
		}
	}

	ordered := make([]string, 0, len(scopes))

	for scope := range scopes {
		ordered = append(ordered, scope)
	}

	return ordered
}

/*
measurementOrigins lists signal registry names stored as tree origin segments.
*/
var measurementOrigins = []string{
	"causal",
	"correlation",
	"cvd",
	"depthflow",
	"exhaust",
	"fluid",
	"hawkes",
	"leadlag",
	"liquidity",
	"manifold",
	"pumpdump",
	"resonance",
	"sentiment",
	"toxicity",
}

func measurementTreePrefix(scope, origin string) []byte {
	return []byte("measurement/" + scope + "/" + origin)
}

/*
collectMeasurementsFromTree queries measurement/<scope>/<origin> rows into story.
*/
func (crypto *Crypto) collectMeasurementsFromTree(scopes []string) {
	if crypto == nil || crypto.tree == nil || crypto.story == nil {
		return
	}

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		for _, origin := range measurementOrigins {
			for inbound := range crypto.tree.Seek(measurementTreePrefix(scope, origin)) {
				crypto.ingestMeasurementArtifact(origin, inbound)
			}
		}
	}
}

func (crypto *Crypto) ingestMeasurementArtifact(origin string, artifact *datura.Artifact) {
	if artifact == nil || crypto.story == nil {
		return
	}

	measurement, ok := logic.MeasurementFromArtifact(origin, artifact)

	if !ok {
		return
	}

	if measurement.Symbol == "" {
		measurement.Symbol, _ = artifact.Scope()
	}

	if measurement.Symbol == "" || measurement.Source == "" {
		return
	}

	errnie.Error(crypto.story.Update(artifact))
}

func (crypto *Crypto) ingestResonanceMeasurements(scopes []string) {
	if crypto.resonance == nil || crypto.story == nil {
		return
	}

	results, settleErr := crypto.resonance.SettleScopes(scopes)

	errnie.Error(settleErr)

	for _, measurement := range results {
		if measurement.Source == "" || measurement.Symbol == "" {
			continue
		}

		artifact := measurementArtifact(measurement)

		if artifact == nil {
			continue
		}

		crypto.ingestMeasurementArtifact("resonance", artifact)
	}
}

func measurementArtifact(measurement logic.Measurement) *datura.Artifact {
	categoryIndex := resonanceClassifierIndex(measurement.Category)

	if categoryIndex <= 0 ||
		!logic.ScalarFinite(measurement.Confidence) ||
		measurement.Confidence <= 0 ||
		measurement.Symbol == "" {
		return nil
	}

	artifact := datura.Acquire("resonance", datura.Artifact_Type_json)

	if artifact == nil {
		return nil
	}

	artifact.WithRole("measurement")
	artifact.WithScope(measurement.Symbol)
	artifact.WithAttribute("classifier.category", categoryIndex)
	artifact.WithAttribute("classifier.confidence", measurement.Confidence)
	artifact.WithAttribute("classifier.strength", measurement.Strength)
	artifact.WithAttribute("price", measurement.Price)
	artifact.WithAttribute("volume", measurement.Volume)
	artifact.WithAttribute("spread", measurement.Spread)
	artifact.WithAttribute("elapsed", measurement.Elapsed)
	artifact.WithAttribute("surprise", measurement.Surprise)

	observedAt := measurement.ObservedAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	artifact.WithAttribute("observed_at", observedAt.UTC().Format(time.RFC3339Nano))

	payload, err := json.Marshal(measurement)

	if err != nil {
		artifact.Release()

		return nil
	}

	if artifact.WithPayload(payload) == nil {
		artifact.Release()

		return nil
	}

	return artifact
}

func resonanceClassifierIndex(category logic.CategoryType) int {
	switch string(category) {
	case resonance.CategoryFlow:
		return 1
	case resonance.CategoryStress:
		return 2
	case resonance.CategoryCoupling:
		return 3
	default:
		return 0
	}
}
