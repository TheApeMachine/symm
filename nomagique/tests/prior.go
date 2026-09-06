package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math/rand"
	"testing"
)

// CheckPrior compares updates and read-only aging to the supplied prior, over
// deterministic weighted observations, zero authority and dormant epochs.
func CheckPrior(t *testing.T, node core.Primitive, memory float64) {
	t.Helper()
	reference := newReferencePrior(memory)
	random := rand.New(rand.NewSource(83))
	for index := 0; index < 400; index++ {
		value := random.NormFloat64()
		authority := random.Float64()
		if index%11 == 0 {
			authority = 0
		}
		event := map[string]any{"value": value, "authority": authority}
		if index < 200 {
			epoch := uint64(index + 1)
			event["epoch"] = epoch
			if err := reference.Observe(value, authority, epoch); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := reference.Observe(value, authority); err != nil {
				t.Fatal(err)
			}
		}
		output := Drain(t, node, Values(Record(event)))
		Sound(t, node)
		comparePrior(t, output, reference.Reading())
	}
	for _, epoch := range []uint64{200, 300, 1000000, 1000000} {
		output := Drain(t, node, Values(Record(map[string]any{"epoch": epoch})))
		Sound(t, node)
		comparePrior(t, output, reference.Reading(epoch))
	}
	event := map[string]any{"value": 7.0, "authority": 0.5, "epoch": uint64(1000001)}
	reference.Observe(7, 0.5, 1000001)
	comparePrior(t, Drain(t, node, Values(Record(event))), reference.Reading())
}
func comparePrior(t *testing.T, output []any, want referencePriorReading) {
	t.Helper()
	if len(output) != 1 {
		t.Fatalf("prior yielded %d records", len(output))
	}
	fields := Fields(t, output[0])
	for key, expected := range map[string]float64{
		"mean": want.Mean, "variance": want.Variance, "support": want.Support,
		"maturity": want.Maturity, "evidence_authority": want.EvidenceAuthority,
		"authority": want.Authority, "memory": want.Memory,
	} {
		EqualNumber(t, Number(t, fields, key), expected)
	}
	if core.To[bool](fields["defined"]) != want.Defined || core.To[bool](fields["variance_defined"]) != want.VarianceDefined {
		t.Fatal("definedness mismatch")
	}
	if core.To[uint64](fields["samples"]) != want.Samples {
		t.Fatal("sample count mismatch")
	}
}
