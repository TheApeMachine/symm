package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

// BoundaryProducer declares storage ownership only. Its coordinates, not its
// name, address the grid. Signal and logic publications have the same contract.
type BoundaryProducer struct {
	Producer    string   `json:"producer"`
	Coordinates []uint32 `json:"coordinates"`
}

// BoundaryRow is the Iceberg row envelope. Float64 values stay in the Cap'n
// Proto payload, so JSON never rounds or substitutes a measurement value.
type BoundaryRow struct {
	Run      string `json:"run"`
	Sequence int64  `json:"sequence"`
	Previous int64  `json:"previous"`
	Digest   string `json:"digest"`
	Payload  []byte `json:"payload"`
}

type boundaryProjection struct {
	layout  string
	values  []float64
	present []bool
}

func boundaryError(message string, cause error) error {
	return errnie.Error(errnie.Err(errnie.Validation, "store.boundary: "+message, cause))
}

func boundaryLayout(declaration string) ([]BoundaryProducer, int, string, error) {
	var producers []BoundaryProducer

	if err := json.Unmarshal([]byte(declaration), &producers); err != nil {
		return nil, 0, "", boundaryError("invalid producer layout", err)
	}

	if len(producers) == 0 {
		return nil, 0, "", boundaryError("a producer layout is required", nil)
	}

	owners := make(map[string]bool, len(producers))
	coordinates := make(map[uint32]bool)

	for _, producer := range producers {
		if producer.Producer == "" || owners[producer.Producer] || len(producer.Coordinates) == 0 {
			return nil, 0, "", boundaryError("each producer must have one nonempty coordinate declaration", nil)
		}

		owners[producer.Producer] = true

		for _, coordinate := range producer.Coordinates {
			if coordinates[coordinate] {
				return nil, 0, "", boundaryError(fmt.Sprintf("coordinate %d has two owners", coordinate), nil)
			}

			coordinates[coordinate] = true
		}
	}

	for index := range len(coordinates) {
		if !coordinates[uint32(index)] {
			return nil, 0, "", boundaryError("coordinate layout must cover a contiguous original grid", nil)
		}
	}

	slices.SortFunc(producers, func(left, right BoundaryProducer) int {
		if left.Producer < right.Producer {
			return -1
		}

		if left.Producer > right.Producer {
			return 1
		}

		return 0
	})

	encoded, err := json.Marshal(producers)

	if err != nil {
		return nil, 0, "", boundaryError("encode producer layout", err)
	}

	return producers, len(coordinates), string(encoded), nil
}

// projectBoundary checks the entire boundary before any retained state moves.
// Missing publications fail; a completed publication may contain only Nil.
func projectBoundary(record MeasurementBoundary) (boundaryProjection, error) {
	var projection boundaryProjection

	if record.Version() != 1 || record.Sequence() <= 0 {
		return projection, boundaryError("unsupported version or invalid sequence", nil)
	}

	run, err := record.Run()

	if err != nil || run == "" {
		return projection, boundaryError("missing run identity", err)
	}

	declaration, err := record.Layout()

	if err != nil {
		return projection, boundaryError("read layout", err)
	}

	producers, width, canonical, err := boundaryLayout(declaration)

	if err != nil {
		return projection, err
	}

	publications, err := record.Publications()

	if err != nil {
		return projection, boundaryError("read publications", err)
	}

	if publications.Len() != len(producers) {
		return projection, boundaryError("incomplete producer boundary", nil)
	}

	declared := make(map[string]BoundaryProducer, len(producers))
	seen := make(map[string]bool, len(producers))
	projection.layout = canonical
	projection.values = make([]float64, width)
	projection.present = make([]bool, width)

	for _, producer := range producers {
		declared[producer.Producer] = producer
	}

	for index := range publications.Len() {
		measurement := publications.At(index)
		owner, err := measurement.Producer()

		if err != nil {
			return boundaryProjection{}, boundaryError("read producer identity", err)
		}

		producer, exists := declared[owner]

		if !exists || seen[owner] {
			return boundaryProjection{}, boundaryError("unknown or duplicate producer: "+owner, nil)
		}

		seen[owner] = true

		if err := projectPublication(measurement, run, record.Sequence(), producer, &projection); err != nil {
			return boundaryProjection{}, err
		}
	}

	return projection, nil
}

func projectPublication(measurement data.Measurement, run string, sequence int64, producer BoundaryProducer, projection *boundaryProjection) error {
	observedRun, err := measurement.Run()

	if err != nil || observedRun != run || measurement.Tick() != sequence {
		return boundaryError("mixed run or sequence in producer "+producer.Producer, err)
	}

	coordinates, err := measurement.Coordinates()

	if err != nil {
		return boundaryError("read publication coordinates", err)
	}

	metrics, err := measurement.Metrics()

	if err != nil {
		return boundaryError("read publication metrics", err)
	}

	present, err := measurement.Present()

	if err != nil {
		return boundaryError("read publication presence", err)
	}

	width := len(producer.Coordinates)

	if coordinates.Len() != width || metrics.Len() != width || present.Len() != width {
		return boundaryError("incomplete coordinate publication: "+producer.Producer, nil)
	}

	for index, expected := range producer.Coordinates {
		if coordinates.At(index) != expected {
			return boundaryError("publication changed its declared coordinates", nil)
		}

		projection.values[expected] = metrics.At(index).Raw()
		projection.present[expected] = present.At(index)
	}

	return nil
}

func boundaryDigest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func readBoundary(payload []byte) (MeasurementBoundary, error) {
	message, err := capnp.Unmarshal(payload)

	if err != nil {
		return MeasurementBoundary{}, boundaryError("decode binary boundary", err)
	}

	record, err := ReadRootMeasurementBoundary(message)

	if err != nil {
		return MeasurementBoundary{}, boundaryError("read binary boundary", err)
	}

	return record, nil
}
