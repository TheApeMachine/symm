package data_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
	"time"
)

func TestProjectionNext(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	projection := &data.Projection{Source: "test",
		Identity: func() (string, string, time.Time, time.Time) { return "id", "BTC/USD", at, at },
		Metrics: []data.MetricProjection{
			{Path: []string{"alpha"}, Label: "alpha", Unit: data.UnitRate},
			{Path: []string{"beta"}, Label: "beta", Defined: []string{"beta_defined"}},
		},
		Facts: []data.FactProjection{
			{Name: data.MetadataSupport, Path: []string{"support"}},
			{Name: data.MetadataDivergence, Path: []string{"residual"}},
			{Name: data.MetadataNoiseVariance, Path: []string{"variance"}},
		},
	}
	for _, value := range []float64{2.5, 0, -1} {
		measurement, err := transport.Evaluate[*data.Measurement[float64]](projection, core.Record(map[string]any{
			"alpha": value, "beta_defined": false, "support": 4.0, "residual": 2.0, "variance": 1.0,
		}))
		if err != nil || measurement.Err != nil {
			t.Fatalf("projection: %v / %v", err, measurement.Err)
		}
		if measurement.ID != "id" || measurement.Metrics["alpha"].Raw != value || measurement.Maturity != 0.75 || measurement.SNR != 4 {
			t.Fatalf("incorrect projection: %+v", measurement)
		}
		if _, exists := measurement.Metrics["beta"]; exists {
			t.Fatal("undefined metric became zero")
		}
	}
}

func TestProjectionRejectsMissingFields(t *testing.T) {
	projection := &data.Projection{Metrics: []data.MetricProjection{{Path: []string{"absent"}, Label: "missing"}}}
	measurement := projection.Project(nil)
	if measurement.Err == nil {
		t.Fatal("missing required metric was hidden")
	}
	refusal := errors.New("unmeasurable")
	projection = &data.Projection{Accepted: []string{"accepted"}, Rejection: refusal}
	measurement = projection.Project(core.To[map[string]core.Primitive](core.Record(map[string]any{"accepted": false})))
	if !errors.Is(measurement.Err, refusal) {
		t.Fatal(measurement.Err)
	}
}

// A first moment has undefined sample variance. It must be absent, not zero,
// in the evidence record. Nested record paths keep this declaration explicit.
func TestProjectionColdStartFacts(t *testing.T) {
	p := &data.Projection{Facts: []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"moments", "count"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"moments", "variance"}, Defined: []string{"moments", "variance_defined"}},
	}}
	fields := map[string]core.Primitive{"moments": core.Record(map[string]any{"count": 1.0, "variance_defined": false})}
	m := p.Project(fields)
	if m.Err != nil || m.Maturity != 0 {
		t.Fatalf("cold start: %+v", m)
	}
	if _, exists := m.Metadata[data.MetadataNoiseVariance]; exists {
		t.Fatal("undefined variance projected")
	}
	fields["moments"] = core.Record(map[string]any{"count": 2.0, "variance_defined": true, "variance": 0.0})
	m = p.Project(fields)
	if m.Err != nil {
		t.Fatal(m.Err)
	}
	if value, exists := m.Metadata[data.MetadataNoiseVariance]; !exists || value != 0 {
		t.Fatal("observed zero variance lost")
	}
}
