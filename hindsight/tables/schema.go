/*
Package tables defines Hindsight's Iceberg table layout, the catalog connection
that owns it, and the encoders that move records in and out.

Each record family Hindsight persists is one Iceberg table. Records were
previously newline-delimited JSON objects grouped under a key prefix; the
column layout here carries the same information, flattened so a reader can
prune on run and time without decoding a payload.

Two deliberate narrowings:

  - Iceberg has no unsigned integer types, so every uint64 in the Go records
    (CaptureSequence, StreamEpoch, ComponentStateVersion, Decision.ID) is
    stored as a signed long. These are monotonic counters that never approach
    2^63, but the narrowing is real.
  - Monetary values are decimal(38,18) rather than float or string. The Kraken
    decimal is an arbitrary-precision unscaled integer plus a scale, which is
    the representation Iceberg's decimal uses, so values round-trip exactly and
    stay aggregable.

This package deliberately imports nothing from hindsight. hindsight and
strategy both need to READ these tables, so a dependency in that direction
would close a cycle; callers convert their own records into the row types
declared here.
*/
package tables

import "github.com/apache/iceberg-go"

/*
Namespace is the Iceberg namespace holding every Hindsight table. The table
bucket it lives in is a separate catalog of its own, selected by the warehouse
location, so the namespace only separates Hindsight from anything else written
into the same bucket.
*/
const Namespace = "hindsight"

// Table names, one per record family.
const (
	Runs      = "runs"
	Captures  = "captures"
	Manifests = "manifests"
	Witnesses = "witnesses"
	Lifecycle = "lifecycle"
	Decisions = "decisions"
	Outcomes  = "outcomes"
	Gaps      = "gaps"
)

/*
Decisions and Outcomes carry the same columns at two different moments.

A decision is recorded when the agent makes it, before the tape has said
anything about it, so its outcome is still null. It is recorded again once a
confirmed leg grades it. They stay separate tables because an append is not an
update: writing the graded row into the same table would leave two rows per
decision and no way to tell the provisional one from the settled one.
*/

/*
There is no separate "states" table.

Records were previously split across a witnesses/ and a states/ key prefix by
whether Artifact.Kind was "state", because a prefix was the only way to make
that distinction cheap to read. It is a column here, so both live in
Witnesses and a reader selects with artifact_kind = 'state'.
*/

/*
DecimalPrecision and DecimalScale size every monetary column. Scale 18 exceeds
anything the venue quotes; precision 38 is Iceberg's ceiling and leaves 20
integer digits.
*/
const (
	DecimalPrecision = 38
	DecimalScale     = 18
)

// moneyType is the column type shared by every monetary value.
func moneyType() iceberg.Type { return iceberg.DecimalTypeOf(DecimalPrecision, DecimalScale) }

/*
envelopeRef is the nested column group naming one Workspace Envelope: the raw
input it came from and its deterministic ordinal within that input. It appears
wherever the Go records embed a hindsight.EnvelopeRef.
*/
func envelopeRef(id int, name string, required bool) iceberg.NestedField {
	return iceberg.NestedField{
		ID: id, Name: name, Required: required,
		Type: &iceberg.StructType{FieldList: []iceberg.NestedField{
			{ID: id*100 + 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
			{ID: id*100 + 2, Name: "sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true},
			{ID: id*100 + 3, Name: "ordinal", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		}},
	}
}

/*
RunsSchema describes one process capture session, carrying enough execution
context to make the rest of the run interpretable.
*/
func RunsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 2, Name: "started_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 3, Name: "code_commit", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 4, Name: "build_id", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 5, Name: "config_digest", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 6, Name: "integrity", Type: iceberg.PrimitiveTypes.String, Required: true,
			Doc: "complete, gapped, or corrupt: the capture-integrity state the run exposes."},
		iceberg.NestedField{ID: 7, Name: "positions", Type: iceberg.PrimitiveTypes.Int32, Required: true,
			Doc: "Positions actually held during the run, so a reader can tell a run that traded from one that did not."},
		// The map's values are declared optional. A required value renders as a
		// non-nullable Arrow map item, which does not match what the table's
		// own writer expects, and the mismatch only surfaces on append.
		iceberg.NestedField{ID: 8, Name: "schema_versions", Required: false,
			Type: &iceberg.MapType{
				KeyID: 801, KeyType: iceberg.PrimitiveTypes.String,
				ValueID: 802, ValueType: iceberg.PrimitiveTypes.String, ValueRequired: false,
			}},
	)
}

/*
CapturesSchema describes one raw external input exactly as it arrived, before
parsing.

CaptureIdentity is flattened rather than nested: run and sequence are what
every reader filters and orders on, and nesting would put them behind a
projection for no gain. Payload stays opaque binary, since its shape depends on
the endpoint that produced it.
*/
func CapturesSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 2, Name: "sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true,
			Doc: "Run-local order in which SYMM observed the input. Primary ordering for causal replay."},
		iceberg.NestedField{ID: 3, Name: "stream", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "stream_epoch", Type: iceberg.PrimitiveTypes.Int64, Required: true,
			Doc: "Connection span within the stream; a reconnect yields a new epoch."},
		iceberg.NestedField{ID: 5, Name: "stream_sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 6, Name: "received_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true,
			Doc: "When SYMM received the frame. Not venue time."},
		iceberg.NestedField{ID: 7, Name: "endpoint", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 8, Name: "kind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 9, Name: "payload_hash", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 10, Name: "payload", Type: iceberg.PrimitiveTypes.Binary},
	)
}

/*
ManifestsSchema records how one raw frame entered Workspace: which workload
processed it, as what domain kind, and the venue time and sequence where the
protocol supplied them.
*/
func ManifestsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		envelopeRef(2, "envelope", true),
		iceberg.NestedField{ID: 3, Name: "workload", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "domain_kind", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 5, Name: "symbol", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 6, Name: "venue_at", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 7, Name: "venue_sequence", Type: iceberg.PrimitiveTypes.String},
	)
}

/*
WitnessesSchema is evidence of what the running binary actually produced at one
Workspace boundary. Parent references are kept structured rather than encoded
into strings so a reader can join on them directly.
*/
func WitnessesSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		envelopeRef(2, "envelope", true),
		iceberg.NestedField{ID: 3, Name: "boundary", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 4, Name: "artifact_kind", Type: iceberg.PrimitiveTypes.String,
			Doc: "ArtifactID.Kind: the artifact family."},
		iceberg.NestedField{ID: 5, Name: "artifact_identity", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 6, Name: "artifact_kind_label", Type: iceberg.PrimitiveTypes.String,
			Doc: "ArtifactWitness.ArtifactKind, which is diagnostic and distinct from ArtifactID.Kind."},
		iceberg.NestedField{ID: 7, Name: "produced_at", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 8, Name: "component", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 9, Name: "component_state_version", Type: iceberg.PrimitiveTypes.Int64},
		iceberg.NestedField{ID: 10, Name: "immediate_parents", Required: false,
			Type: &iceberg.ListType{
				ElementID: 1010, ElementRequired: true,
				Element: &iceberg.StructType{FieldList: []iceberg.NestedField{
					{ID: 1011, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
					{ID: 1012, Name: "sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true},
					{ID: 1013, Name: "ordinal", Type: iceberg.PrimitiveTypes.Int64, Required: true},
				}},
			}},
		iceberg.NestedField{ID: 11, Name: "semantic_parents", Required: false,
			Type: &iceberg.ListType{
				ElementID: 1110, ElementRequired: true,
				Element: iceberg.PrimitiveTypes.String,
			},
			Doc: "Shared semantic inputs consumed whose origin envelope was not the triggering envelope."},
		iceberg.NestedField{ID: 12, Name: "payload", Type: iceberg.PrimitiveTypes.Binary},
	)
}

/*
LifecycleSchema records position and order transitions. Execution economics are
flattened into exec_* columns so they can be aggregated without decoding, and
the per-asset fee breakdown is kept as JSON since it is a rarely-queried tail.
*/
func LifecycleSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 2, Name: "decision_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 3, Name: "action_correlation_id", Type: iceberg.PrimitiveTypes.String,
			Doc: "Client order ID of this instruction. decision_id retains the entry witness and must not price later actions."},
		iceberg.NestedField{ID: 4, Name: "symbol", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "kind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 6, Name: "action", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 7, Name: "at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 8, Name: "capture_seq", Type: iceberg.PrimitiveTypes.Int64,
			Doc: "Capture sequence of the envelope whose decision caused this transition. Zero when no decision witness recorded it."},
		iceberg.NestedField{ID: 9, Name: "exec_order_id", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 10, Name: "exec_client_order_id", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 11, Name: "exec_id", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 12, Name: "exec_type", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 13, Name: "exec_trade_id", Type: iceberg.PrimitiveTypes.Int64},
		iceberg.NestedField{ID: 14, Name: "exec_side", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 15, Name: "exec_order_type", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 16, Name: "exec_order_status", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 17, Name: "exec_liquidity_ind", Type: iceberg.PrimitiveTypes.String},
		iceberg.NestedField{ID: 18, Name: "exec_at", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 19, Name: "exec_last_qty", Type: moneyType()},
		iceberg.NestedField{ID: 20, Name: "exec_last_price", Type: moneyType()},
		iceberg.NestedField{ID: 21, Name: "exec_cost", Type: moneyType()},
		iceberg.NestedField{ID: 22, Name: "exec_cum_qty", Type: moneyType()},
		iceberg.NestedField{ID: 23, Name: "exec_cum_cost", Type: moneyType()},
		iceberg.NestedField{ID: 24, Name: "exec_avg_price", Type: moneyType()},
		iceberg.NestedField{ID: 25, Name: "exec_fee_usd_equiv", Type: moneyType()},
		iceberg.NestedField{ID: 26, Name: "exec_fees", Type: iceberg.PrimitiveTypes.String,
			Doc: "Per-asset fee breakdown as JSON. A rarely-queried tail, kept whole rather than exploded."},
	)
}

/*
DecisionsSchema records one decision as the agent made it, before grading.
*/
func DecisionsSchema() *iceberg.Schema { return OutcomesSchema() }

/*
OutcomesSchema records one graded decision: what the agent chose, under what
authority, and what the tape did afterwards.
*/
func OutcomesSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 2, Name: "decision_id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "trader", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: 4, Name: "label", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 5, Name: "at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: 6, Name: "action_kind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 7, Name: "action_power", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: 8, Name: "action_reduce", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 9, Name: "authority", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 10, Name: "outcome", Type: iceberg.PrimitiveTypes.Float64,
			Doc: "Null until the decision is graded."},
		iceberg.NestedField{ID: 11, Name: "context", Required: false,
			Type: &iceberg.ListType{ElementID: 1110, ElementRequired: true, Element: iceberg.PrimitiveTypes.Int64}},
		iceberg.NestedField{ID: 12, Name: "through", Type: iceberg.PrimitiveTypes.TimestampTz},
		iceberg.NestedField{ID: 13, Name: "value", Type: iceberg.PrimitiveTypes.Float64, Required: true},
		iceberg.NestedField{ID: 14, Name: "complete", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: 15, Name: "forced", Type: iceberg.PrimitiveTypes.Bool, Required: true,
			Doc: "A decision that had no alternative. Its outcome describes the market, not the action, so it is released rather than trained."},
		iceberg.NestedField{ID: 16, Name: "initial", Type: moneyType()},
		iceberg.NestedField{ID: 17, Name: "reference", Type: moneyType()},
		iceberg.NestedField{ID: 18, Name: "quantity", Type: moneyType()},
		iceberg.NestedField{ID: 19, Name: "cost", Type: moneyType()},
		iceberg.NestedField{ID: 20, Name: "fee", Type: moneyType()},
		iceberg.NestedField{ID: 21, Name: "opportunity", Type: moneyType()},
	)
}

/*
GapsSchema records where a run's capture is known to be incomplete: a sequence
with no captured input, or an input observed whose persistence did not commit.
*/
func GapsSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "run", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: 2, Name: "sequence", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 3, Name: "encoding", Type: iceberg.PrimitiveTypes.String, Required: true,
			Doc: "missing_sequence or persistence_failure."},
	)
}
