/*
Package relation measures directed temporal predictive contribution between
Measurement coordinates.

A Relation answers one question:

	Did knowing Source history improve prediction of Target beyond Target's
	own history and explicitly supplied Controls?

Relation measures predictive Influence. It does not prove physical causality,
does not decide actions, does not create market categories, and never deletes
measurements. Feature selection is query-local; the full observational
Coordinate history remains available.

The normative contract is nomagique/relation/README.md. Where this code and
the README disagree, the README wins.
*/
package relation

import (
	"fmt"
	"strings"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Coordinate is the typed identity of one measurement coordinate. Every field
participates in identity, so these remain distinct:

	symbol, peer (if any), signal source, metric, side, unit, timescale,
	model epoch.

Coordinate is a comparable struct and is used directly as the internal map
key. Rendered strings are for logs and telemetry only; identity is never a
colon-joined string.
*/
type Coordinate struct {
	// Symbol is the market symbol, e.g. "BTC/USD".
	Symbol string
	// Peer is the peer symbol when the measurement is bivariate, else empty.
	Peer string
	// Source is the signal source, e.g. "cvd" or "hawkes".
	Source string
	// Metric is the metric name without any side suffix, e.g. "signed_net_fraction".
	Metric string
	// Side is the side suffix when the metric is side-keyed, e.g. "buy".
	Side string
	// Unit is the physical unit of the measurement.
	Unit nmtypes.Unit
	// Timescale is the measurement timescale.
	Timescale nmtypes.Timescale
	// Epoch is the model epoch. Incompatible epochs are never mixed.
	Epoch uint64
}

/*
CompareCoordinate orders two Coordinates field-wise without materializing any
identity string. The lexicographic field order matches Coordinate.ID's render
order (Symbol, Source, Metric, Side, Peer, Unit, Timescale, Epoch), so ordering
and rendered identity agree. It is the allocation-free ordering primitive for
every deterministic sort; computational comparators must never call
Coordinate.ID, which allocates on every invocation.
*/
func CompareCoordinate(left Coordinate, right Coordinate) int {
	if left.Symbol != right.Symbol {
		return strings.Compare(left.Symbol, right.Symbol)
	}

	if left.Source != right.Source {
		return strings.Compare(left.Source, right.Source)
	}

	if left.Metric != right.Metric {
		return strings.Compare(left.Metric, right.Metric)
	}

	if left.Side != right.Side {
		return strings.Compare(left.Side, right.Side)
	}

	if left.Peer != right.Peer {
		return strings.Compare(left.Peer, right.Peer)
	}

	if left.Unit != right.Unit {
		if left.Unit < right.Unit {
			return -1
		}

		return 1
	}

	if left.Timescale != right.Timescale {
		if left.Timescale < right.Timescale {
			return -1
		}

		return 1
	}

	if left.Epoch != right.Epoch {
		if left.Epoch < right.Epoch {
			return -1
		}

		return 1
	}

	return 0
}

/*
ID returns the rendered identity string. It is reversible via ParseCoordinate
and is intended for logs, telemetry, and serialization — never as the sole
internal identity.
*/
func (coordinate Coordinate) ID() string {
	parts := []string{
		coordinate.Symbol,
		coordinate.Source,
		coordinate.Metric,
		coordinate.Side,
	}

	if coordinate.Peer != "" {
		parts = append(parts, "peer="+coordinate.Peer)
	}

	parts = append(parts,
		coordinate.Unit.String(),
		coordinate.Timescale.String(),
		fmt.Sprintf("epoch=%d", coordinate.Epoch),
	)

	return strings.Join(parts, "/")
}

/*
ParseMetricSide splits a projected metric label into its base metric and side
suffix. The signal boundary keys metrics as "metric" or "metric:side", so the
first colon separates the side suffix; namespaced metric names use '/' and are
never split.
*/
func ParseMetricSide(label string) (metric string, side string) {
	if index := strings.IndexByte(label, ':'); index >= 0 {
		return label[:index], label[index+1:]
	}

	return label, ""
}

/*
ParseCoordinate reconstructs a Coordinate from its rendered ID. It is the
inverse of Coordinate.ID for serialization round-trips.
*/
func ParseCoordinate(id string) (Coordinate, error) {
	parts := strings.Split(id, "/")

	if len(parts) < 3 {
		return Coordinate{}, fmt.Errorf("relation: malformed coordinate id %q", id)
	}

	coordinate := Coordinate{
		Symbol: parts[0],
		Source: parts[1],
		Metric: parts[2],
	}

	next := 3

	if next < len(parts) && parts[next] != "" && !strings.HasPrefix(parts[next], "peer=") &&
		!strings.HasPrefix(parts[next], "epoch=") {
		coordinate.Side = parts[next]
		next++
	}

	for _, part := range parts[next:] {
		switch {
		case strings.HasPrefix(part, "peer="):
			coordinate.Peer = strings.TrimPrefix(part, "peer=")
		case strings.HasPrefix(part, "epoch="):
			_, err := fmt.Sscanf(strings.TrimPrefix(part, "epoch="), "%d", &coordinate.Epoch)

			if err != nil {
				return Coordinate{}, fmt.Errorf("relation: malformed epoch in %q: %w", id, err)
			}
		default:
			switch part {
			case nmtypes.UnitDimensionless.String():
				coordinate.Unit = nmtypes.UnitDimensionless
			case nmtypes.UnitCount.String():
				coordinate.Unit = nmtypes.UnitCount
			case nmtypes.UnitRate.String():
				coordinate.Unit = nmtypes.UnitRate
			case nmtypes.UnitDuration.String():
				coordinate.Unit = nmtypes.UnitDuration
			case nmtypes.UnitPrice.String():
				coordinate.Unit = nmtypes.UnitPrice
			case nmtypes.UnitPercent.String():
				coordinate.Unit = nmtypes.UnitPercent
			case nmtypes.UnitQuoteCurrency.String():
				coordinate.Unit = nmtypes.UnitQuoteCurrency
			case nmtypes.UnitBaseCurrency.String():
				coordinate.Unit = nmtypes.UnitBaseCurrency
			case nmtypes.UnitEventsPerSecond.String():
				coordinate.Unit = nmtypes.UnitEventsPerSecond
			case nmtypes.UnitPerSecond.String():
				coordinate.Unit = nmtypes.UnitPerSecond
			case nmtypes.UnitInverseSecond.String():
				coordinate.Unit = nmtypes.UnitInverseSecond
			case nmtypes.UnitNat.String():
				coordinate.Unit = nmtypes.UnitNat
			case nmtypes.UnitSecond.String():
				coordinate.Unit = nmtypes.UnitSecond
			default:
				coordinate.Timescale = nmtypes.ParseTimescale(part)
			}
		}
	}

	return coordinate, nil
}
