package ui

import (
	"context"
	"fmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
)

// inspection resolves normalized table references to their recorded capture
// identities for one HTTP response. Its cache is request-local, never live state.
type inspection struct {
	ctx      context.Context
	catalog  *tables.Catalog
	captures map[tables.EnvelopeRefRow]hindsight.CaptureIdentity
}

func newInspection(ctx context.Context, catalog *tables.Catalog) *inspection {
	return &inspection{ctx: ctx, catalog: catalog, captures: make(map[tables.EnvelopeRefRow]hindsight.CaptureIdentity)}
}

func (inspection *inspection) reference(row tables.EnvelopeRefRow) (hindsight.EnvelopeRef, error) {
	key := tables.EnvelopeRefRow{Run: row.Run, Sequence: row.Sequence}
	identity, known := inspection.captures[key]

	if !known {
		capture, found, err := inspection.catalog.Capture(inspection.ctx, row.Run, row.Sequence)
		if err != nil {
			return hindsight.EnvelopeRef{}, errnie.Error(err)
		}
		if !found {
			return hindsight.EnvelopeRef{}, errnie.Error(errnie.Err(errnie.NotFound,
				fmt.Sprintf("hindsight: dangling capture reference %s/%d", row.Run, row.Sequence), nil))
		}
		identity = hindsight.FrameFromRow(capture).Identity
		inspection.captures[key] = identity
	}
	return hindsight.EnvelopeRef{Origin: identity, Ordinal: uint64(row.Ordinal)}, nil
}

func (inspection *inspection) witness(row tables.WitnessRow) (hindsight.ArtifactWitness, error) {
	envelope, err := inspection.reference(row.Envelope)
	if err != nil {
		return hindsight.ArtifactWitness{}, errnie.Error(err)
	}
	result := hindsight.ArtifactWitness{
		Envelope: envelope, Boundary: row.Boundary,
		Artifact:     hindsight.ArtifactID{Kind: row.ArtifactKind, Identity: row.ArtifactIdentity},
		ArtifactKind: row.ArtifactKindLabel, ProducedAt: row.ProducedAt,
		Component: row.Component, ComponentStateVersion: uint64(row.ComponentStateVersion),
		SemanticParents: row.SemanticParents, Payload: row.Payload,
	}
	for _, parent := range row.ImmediateParents {
		reference, err := inspection.reference(parent)
		if err != nil {
			return hindsight.ArtifactWitness{}, errnie.Error(err)
		}
		result.ImmediateParents = append(result.ImmediateParents, reference)
	}
	return result, nil
}

func (inspection *inspection) manifest(row tables.ManifestRow) (hindsight.EnvelopeManifest, error) {
	envelope, err := inspection.reference(row.Envelope)
	if err != nil {
		return hindsight.EnvelopeManifest{}, errnie.Error(err)
	}
	return hindsight.EnvelopeManifest{Envelope: envelope, Workload: row.Workload,
		DomainKind: row.DomainKind, Symbol: row.Symbol, VenueAt: row.VenueAt, VenueSequence: row.VenueSequence}, nil
}
