package manifold

import (
	"context"
	"io"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	feed "github.com/theapemachine/symm/signal"
)

type Features struct {
	ctx    context.Context
	cancel context.CancelFunc
	scope  string
	field  *Field
}

func NewFeatures(ctx context.Context, field *Field) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:    ctx,
		cancel: cancel,
		field:  field,
	}
}

func (features *Features) Read(p []byte) (int, error) {
	reading, price, _, ok := features.field.Reading(features.scope)

	if !ok || !reading.IsFinite() {
		return 0, io.EOF
	}

	artifact := datura.Acquire("manifold-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(
		reading.PressureGradNorm,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
		price,
	))

	return artifact.Read(p)
}

func manifoldFeedError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "non-finite") {
		return nil
	}

	return errnie.Error(err)
}
