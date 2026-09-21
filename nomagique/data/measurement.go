package data

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Measurement is the native data type in nomagique, and in most cases should be
leveraged to build a system that is easy to work with, because of a mostly
mono-typed architecture.

It implements Identifiable, so it can work with nomagique stores that use index
addressable storage slots for O(1) reads. The register yields a working copy of
a slot; published snapshots that appear as peers are not written.

Label is often used as a named canonical group that makes sense in a given
project, while SeqIdx is the workspace observation index used as a
synchronization anchor. Zero means the observation has not been stamped.

At should generally always be set to the timestamp of the event that fills
the Measurement, while From is optional, but has value beyond just defining
a window of time for the measurement. It can also be used to derive rudimentary
performance and latency diagnostics.

Quality is not caller-supplied. Maturity and SNR are derived by the Finalizer
primitive from the measurement's own estimator facts, these values represent
the amount of trust to put in the overal Measurement, and the Metrics it contains.
*/
type Measurement[T any] struct {
	ID         T                    `json:"id"`
	Label      string               `json:"label"`
	Source     string               `json:"source"`
	SeqIdx     int64                `json:"seqIdx"`
	Timestamp  int64                `json:"timestamp"`
	At         time.Time            `json:"at"`
	From       time.Time            `json:"from,omitempty"`
	Maturity   float64              `json:"maturity"`
	SNR        float64              `json:"snr"`
	SNRDefined bool                 `json:"snrDefined"`
	Estimated  bool                 `json:"estimated"`
	Err        error                `json:"-"`
	Metrics    map[string]Metric[T] `json:"metrics,omitempty"`
	Metadata   map[string]string    `json:"metadata,omitempty"`
	Provenance map[string]string    `json:"provenance,omitempty"`
	Peers      []*Measurement[T]    `json:"peers"`
	// Result is the completed, immutable structured output of this observation.
	// The register and tees share it; numeric persistence uses Metrics.
	Result any `json:"-"`
}

/*
NewMeasurement creates one identified observation with empty metric storage.
Its identity is unstamped: the register slot that owns it is assigned when a
workload registers it, and the node stamps the slot back via SetID.
*/
func NewMeasurement[T any](
	source string, metrics map[string]Metric[T],
) *Measurement[T] {
	if metrics == nil {
		metrics = make(map[string]Metric[T])
	}

	return &Measurement[T]{
		Source:     source,
		Metrics:    metrics,
		Metadata:   make(map[string]string),
		Provenance: make(map[string]string),
	}
}

/*
Identity names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identity() T {
	return measurement.ID
}

/*
Identify names the register slot this measurement's consumer node owns.
*/
func (measurement *Measurement[T]) Identify(id T) T {
	measurement.ID = id
	return measurement.ID
}

/*
FindPeer returns the first peer matching the given predicate, or nil if none match.
*/
func (measurement *Measurement[T]) FindPeer(predicate func(*Measurement[T]) bool) *Measurement[T] {
	if measurement == nil || predicate == nil {
		return nil
	}

	for _, peer := range measurement.Peers {
		if peer != nil && predicate(peer) {
			return peer
		}
	}

	return nil
}

/*
Clone returns an independent copy of the measurement and its mappings. Peer
pointers are copied, not cloned: the register attaches live published snapshots.
*/
func (measurement *Measurement[T]) Clone() *Measurement[T] {
	if measurement == nil {
		return nil
	}

	metrics := make(map[string]Metric[T], len(measurement.Metrics))
	maps.Copy(metrics, measurement.Metrics)

	var metadata map[string]string

	if measurement.Metadata != nil {
		metadata = make(map[string]string, len(measurement.Metadata))
		maps.Copy(metadata, measurement.Metadata)
	}

	var provenance map[string]string

	if measurement.Provenance != nil {
		provenance = make(map[string]string, len(measurement.Provenance))
		maps.Copy(provenance, measurement.Provenance)
	}

	var peers []*Measurement[T]

	if len(measurement.Peers) != 0 {
		peers = make([]*Measurement[T], len(measurement.Peers))
		copy(peers, measurement.Peers)
	}

	return &Measurement[T]{
		ID:         measurement.ID,
		Label:      measurement.Label,
		Source:     measurement.Source,
		SeqIdx:     measurement.SeqIdx,
		Timestamp:  measurement.Timestamp,
		At:         measurement.At,
		From:       measurement.From,
		Maturity:   measurement.Maturity,
		SNR:        measurement.SNR,
		SNRDefined: measurement.SNRDefined,
		Estimated:  measurement.Estimated,
		Err:        measurement.Err,
		Metrics:    metrics,
		Metadata:   metadata,
		Provenance: provenance,
		Peers:      peers,
		Result:     measurement.Result,
	}
}

/*
Pull copies event identity, provenance, and the named metrics from other onto
this measurement. The source is not mutated. Identity, source, and metadata
stay with this measurement.
*/
func (measurement *Measurement[T]) Pull(other *Measurement[T], keys ...string) {
	if measurement == nil || other == nil {
		return
	}

	measurement.Label = other.Label
	measurement.At = other.At
	measurement.From = other.From
	measurement.SeqIdx = other.SeqIdx
	measurement.Timestamp = other.Timestamp

	if other.Provenance != nil {
		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string, len(other.Provenance))
		}

		maps.Copy(measurement.Provenance, other.Provenance)
	}

	if len(keys) == 0 {
		return
	}

	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]Metric[T], len(keys))
	}

	for _, key := range keys {
		if metric, ok := other.Metrics[key]; ok {
			measurement.Metrics[key] = metric
		}
	}
}

/*
Reset zeroes every metric's values in place, so a pre-allocated measurement
can flow through again without being reallocated. The declared schema never
moves.
*/
func (measurement *Measurement[T]) Reset() {
	for key, metric := range measurement.Metrics {
		metric.Raw = zero[T]()
		metric.Normalized = nil
		metric.Standardized = nil
		measurement.Metrics[key] = metric
	}
}

/*
zero is the type's zero value, for clearing a metric's observation.
*/
func zero[T any]() T {
	var value T

	return value
}

const (
	MetadataSupport        = "support"
	MetadataMaturity       = "maturity"
	MetadataDivergence     = "divergence"
	MetadataNoiseVariance  = "noise_variance"
	MetadataMahalanobisSNR = "mahalanobis_snr"
)

/*
Finalize derives the measurement's quality facts from its own estimator
metadata, mutating the measurement in place.
*/
func (measurement *Measurement[Value]) Finalize() {
	finalizer := NewFinalizer[Value]()
	finalizer(measurement)
}

/*
NewFinalizer creates the measurement quality derivation Value closure.
No structs, pure Value closure.
*/
type Finalizer[Value any] func(*Measurement[Value]) *Measurement[Value]

func NewFinalizer[Value any]() Finalizer[Value] {
	server := NewQuality()

	return func(measurement *Measurement[Value]) *Measurement[Value] {
		if measurement != nil {
			var support, divergence, noiseVariance, mahalanobisSNR, maturity float64
			if val, ok := measurement.Metadata["support"]; ok {
				support, _ = strconv.ParseFloat(val, 64)
			}
			if val, ok := measurement.Metadata["divergence"]; ok {
				divergence, _ = strconv.ParseFloat(val, 64)
			}
			if val, ok := measurement.Metadata["noise_variance"]; ok {
				noiseVariance, _ = strconv.ParseFloat(val, 64)
			}
			if val, ok := measurement.Metadata["mahalanobis_snr"]; ok {
				mahalanobisSNR, _ = strconv.ParseFloat(val, 64)
			}
			if val, ok := measurement.Metadata["maturity"]; ok {
				maturity, _ = strconv.ParseFloat(val, 64)
			}

			client := Quality_ServerToClient(server)
			ctx := context.Background()

			_ = client.Write(ctx, func(p Quality_write_Params) error {
				p.SetSupport(support)
				p.SetDivergence(divergence)
				p.SetNoiseVariance(noiseVariance)
				p.SetMahalanobisSNR(mahalanobisSNR)
				p.SetMaturity(maturity)
				return nil
			})
			_ = client.WaitStreaming()

			call, release := client.Done(ctx, nil)
			defer release()

			res, err := call.Struct()
			if err == nil {
				measurement.Maturity = res.Maturity()
				measurement.SNR = res.Snr()
				measurement.SNRDefined = res.SnrDefined()
				measurement.Estimated = res.Estimated()
			}
		}

		return measurement
	}
}

/*
MarshalCapnp serializes the measurement into authoritative Cap'n Proto WireMeasurement binary framing.
*/
func (measurement *Measurement[T]) MarshalCapnp() ([]byte, error) {
	if measurement == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"data.measurement: cannot marshal nil measurement",
			nil,
		))
	}

	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"data.measurement: failed to allocate capnp message",
			err,
		))
	}

	wire, err := NewRootWireMeasurement(seg)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"data.measurement: failed to allocate root wire measurement",
			err,
		))
	}

	idStr := fmt.Sprint(measurement.ID)
	_ = wire.SetId([]byte(idStr))
	_ = wire.SetLabel([]byte(measurement.Label))
	wire.SetTick(measurement.SeqIdx)
	wire.SetTimestamp(measurement.Timestamp)

	if measurement.At.UnixNano() > 0 {
		wire.SetEpoch(measurement.At.UnixNano())
	}

	wire.SetSource(parseSourceType(measurement.Source))
	wire.SetSnr(measurement.SNR)
	wire.SetMaturity(measurement.Maturity)

	if len(measurement.Metrics) > 0 {
		metricsList, err := wire.NewMetrics(int32(len(measurement.Metrics)))

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				"data.measurement: failed to allocate metrics list",
				err,
			))
		}

		keys := make([]string, 0, len(measurement.Metrics))
		for metricKey := range measurement.Metrics {
			keys = append(keys, metricKey)
		}
		sort.Strings(keys)

		for index, metricKey := range keys {
			metricItem := measurement.Metrics[metricKey]
			item := metricsList.At(index)
			item.SetRaw(toFloat64(metricItem.Raw))

			if metricItem.Normalized != nil {
				item.SetNormalized(toFloat64(*metricItem.Normalized))
			}

			if metricItem.Standardized != nil {
				item.SetStandardized(toFloat64(*metricItem.Standardized))
			}
		}
	}

	if len(measurement.Metadata) > 0 {
		wireMap, err := wire.NewMetadata()

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				"data.measurement: failed to allocate metadata wire map",
				err,
			))
		}

		entriesList, err := wireMap.NewEntries(int32(len(measurement.Metadata)))

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				"data.measurement: failed to allocate metadata entries list",
				err,
			))
		}

		metaKeys := make([]string, 0, len(measurement.Metadata))
		for metaKey := range measurement.Metadata {
			metaKeys = append(metaKeys, metaKey)
		}
		sort.Strings(metaKeys)

		for index, metaKey := range metaKeys {
			entry := entriesList.At(index)
			textPtr, err := capnp.NewText(seg, metaKey)

			if err == nil {
				_ = entry.SetKey(textPtr.ToPtr())
			}

			valStr := measurement.Metadata[metaKey]
			metadataVal, err := NewMetadataValue(seg)

			if err == nil {
				_ = metadataVal.SetText(valStr)
				_ = entry.SetValue(metadataVal.ToPtr())
			}
		}
	}

	return msg.Marshal()
}

/*
UnmarshalMeasurement decodes a Cap'n Proto WireMeasurement binary payload into an identified Measurement.
*/
func UnmarshalMeasurement(payload []byte) (*Measurement[string], error) {
	if len(payload) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"data.measurement: empty payload",
			nil,
		))
	}

	msg, err := capnp.Unmarshal(payload)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"data.measurement: unmarshal capnp message failed",
			err,
		))
	}

	wire, err := ReadRootWireMeasurement(msg)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"data.measurement: read root wire measurement failed",
			err,
		))
	}

	idBytes, _ := wire.Id()
	labelBytes, _ := wire.Label()

	m := &Measurement[string]{
		ID:         string(idBytes),
		Label:      string(labelBytes),
		Source:     formatSourceType(wire.Source()),
		SeqIdx:     wire.Tick(),
		Timestamp:  wire.Timestamp(),
		Maturity:   wire.Maturity(),
		SNR:        wire.Snr(),
		SNRDefined: wire.Snr() > 0,
		Metrics:    make(map[string]Metric[string]),
		Metadata:   make(map[string]string),
		Provenance: make(map[string]string),
	}

	if wire.Epoch() > 0 {
		m.At = time.Unix(0, wire.Epoch())
	}

	metricsList, err := wire.Metrics()

	if err == nil && metricsList.Len() > 0 {
		count := metricsList.Len()

		for index := 0; index < count; index++ {
			item := metricsList.At(index)
			key := fmt.Sprintf("metric_%d", index)
			rawVal := item.Raw()
			normVal := item.Normalized()
			stdVal := item.Standardized()

			m.Metrics[key] = Metric[string]{
				Raw: fmt.Sprintf("%f", rawVal),
			}

			if normVal != 0 {
				normStr := fmt.Sprintf("%f", normVal)
				m.Metrics[key] = Metric[string]{
					Raw:        fmt.Sprintf("%f", rawVal),
					Normalized: &normStr,
				}
			}

			if stdVal != 0 {
				stdStr := fmt.Sprintf("%f", stdVal)
				curr := m.Metrics[key]
				curr.Standardized = &stdStr
				m.Metrics[key] = curr
			}
		}
	}

	wireMap, err := wire.Metadata()

	if err == nil && wireMap.IsValid() {
		entries, err := wireMap.Entries()

		if err == nil && entries.Len() > 0 {
			count := entries.Len()

			for index := 0; index < count; index++ {
				entry := entries.At(index)
				keyPtr, err := entry.Key()

				if err != nil || !keyPtr.IsValid() {
					continue
				}

				keyStr := keyPtr.Text()
				valPtr, err := entry.Value()

				if err != nil || !valPtr.IsValid() {
					continue
				}

				valStruct := MetadataValue(valPtr.Struct())
				which := valStruct.Which()

				switch which {
				case MetadataValue_Which_text:
					textVal, err := valStruct.Text()

					if err == nil {
						m.Metadata[keyStr] = textVal
					}
				case MetadataValue_Which_int:
					m.Metadata[keyStr] = strconv.FormatInt(valStruct.Int(), 10)
				case MetadataValue_Which_float:
					m.Metadata[keyStr] = strconv.FormatFloat(valStruct.Float(), 'f', -1, 64)
				case MetadataValue_Which_bool:
					m.Metadata[keyStr] = strconv.FormatBool(valStruct.Bool())
				case MetadataValue_Which_id:
					dataVal, err := valStruct.Id()

					if err == nil {
						m.Metadata[keyStr] = string(dataVal)
					}
				}
			}
		}
	}

	return m, nil
}

func parseSourceType(source string) WireMeasurement_SourceType {
	switch strings.ToLower(source) {
	case "public":
		return WireMeasurement_SourceType_public
	case "private":
		return WireMeasurement_SourceType_private
	case "level3":
		return WireMeasurement_SourceType_level3
	case "correlation":
		return WireMeasurement_SourceType_correlation
	case "csv":
		return WireMeasurement_SourceType_csv
	case "depthflow":
		return WireMeasurement_SourceType_depthflow
	case "derivatives":
		return WireMeasurement_SourceType_derivatives
	case "hawkes":
		return WireMeasurement_SourceType_hawkes
	case "leadlag":
		return WireMeasurement_SourceType_leadlag
	case "liquidity":
		return WireMeasurement_SourceType_liquidity
	case "morphology":
		return WireMeasurement_SourceType_morphology
	case "pumpdump":
		return WireMeasurement_SourceType_pumpdump
	case "sentiment":
		return WireMeasurement_SourceType_sentiment
	case "toxicity":
		return WireMeasurement_SourceType_toxicity
	case "category":
		return WireMeasurement_SourceType_category
	case "cognition":
		return WireMeasurement_SourceType_cognition
	case "resonance":
		return WireMeasurement_SourceType_resonance
	case "manifold":
		return WireMeasurement_SourceType_manifold
	case "training":
		return WireMeasurement_SourceType_training
	default:
		return WireMeasurement_SourceType_public
	}
}

func formatSourceType(source WireMeasurement_SourceType) string {
	switch source {
	case WireMeasurement_SourceType_public:
		return "public"
	case WireMeasurement_SourceType_private:
		return "private"
	case WireMeasurement_SourceType_level3:
		return "level3"
	case WireMeasurement_SourceType_correlation:
		return "correlation"
	case WireMeasurement_SourceType_csv:
		return "csv"
	case WireMeasurement_SourceType_depthflow:
		return "depthflow"
	case WireMeasurement_SourceType_derivatives:
		return "derivatives"
	case WireMeasurement_SourceType_hawkes:
		return "hawkes"
	case WireMeasurement_SourceType_leadlag:
		return "leadlag"
	case WireMeasurement_SourceType_liquidity:
		return "liquidity"
	case WireMeasurement_SourceType_morphology:
		return "morphology"
	case WireMeasurement_SourceType_pumpdump:
		return "pumpdump"
	case WireMeasurement_SourceType_sentiment:
		return "sentiment"
	case WireMeasurement_SourceType_toxicity:
		return "toxicity"
	case WireMeasurement_SourceType_category:
		return "category"
	case WireMeasurement_SourceType_cognition:
		return "cognition"
	case WireMeasurement_SourceType_resonance:
		return "resonance"
	case WireMeasurement_SourceType_manifold:
		return "manifold"
	case WireMeasurement_SourceType_training:
		return "training"
	default:
		return "public"
	}
}

func toFloat64(val any) float64 {
	switch number := val.(type) {
	case float64:
		return number
	case float32:
		return float64(number)
	case int:
		return float64(number)
	case int64:
		return float64(number)
	case string:
		parsed, err := strconv.ParseFloat(number, 64)

		if err != nil {
			return 0
		}

		return parsed
	default:
		return 0
	}
}

