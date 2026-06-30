package trader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/playbook/optimizer"
)

func TestMergeDecisionRecordCarriesBackendEconomics(t *testing.T) {
	action := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("ETH/USD").
		WithAttribute("type", "limit").
		WithAttribute("decision.confidence", 0.8).
		WithAttribute("decision.score", 0.7).
		WithAttribute("decision.edge", 0.012).
		WithAttribute("decision.hurdle", 0.0069).
		WithAttribute("decision.friction", 0.0069).
		WithAttribute("decision.economic_priced", true).
		WithAttribute("execution.liquidity", "maker").
		WithAttribute("allowed", true)

	records := map[string]datura.Map[any]{}
	(&Crypto{}).mergeDecisionRecord(records, 42, action, "")

	record, ok := records["ETH/USD"]
	if !ok {
		t.Fatalf("decision record missing: %#v", records)
	}
	if record["edge"] != 0.012 {
		t.Fatalf("edge = %#v, want 0.012", record["edge"])
	}
	if record["hurdle"] != 0.0069 {
		t.Fatalf("hurdle = %#v, want 0.0069", record["hurdle"])
	}
	if record["friction"] != 0.0069 {
		t.Fatalf("friction = %#v, want 0.0069", record["friction"])
	}
	if record["economic_priced"] != true {
		t.Fatalf("economic_priced = %#v, want true", record["economic_priced"])
	}
	if record["liquidity"] != "maker" {
		t.Fatalf("liquidity = %#v, want maker", record["liquidity"])
	}
}

func TestWriteDecisionAuditProducesOptimizerInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	recorder, err := audit.NewRecorder(path)
	if err != nil {
		t.Fatalf("new audit recorder: %v", err)
	}

	crypto := &Crypto{audit: recorder}
	crypto.writeDecisionAudit(datura.Map[any]{
		"tick": int64(42),
		"decisions": []datura.Map[any]{
			{
				"symbol":          "ETH/USD",
				"type":            "limit",
				"source":          "fluid",
				"category":        "laminar",
				"confidence":      0.8,
				"edge":            0.012,
				"friction":        0.0069,
				"economic_priced": true,
			},
		},
	})
	if err := recorder.Close(); err != nil {
		t.Fatalf("close audit recorder: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	defer file.Close()

	samples, err := optimizer.ReadJSONL(file)
	if err != nil {
		t.Fatalf("optimizer read audit jsonl: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Source != "fluid" || samples[0].Category != "laminar" {
		t.Fatalf("sample did not preserve decision evidence: %#v", samples[0])
	}
}

func TestCandidateDecisionAuditWritesOneRowPerCandidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	recorder, err := audit.NewRecorder(path)
	if err != nil {
		t.Fatalf("new audit recorder: %v", err)
	}

	first := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("ETH/USD").
		WithAttribute("type", "limit").
		WithAttribute("reason_source", "fluid").
		WithAttribute("reason_category", "laminar").
		WithAttribute("decision.confidence", 0.8).
		WithAttribute("decision.score", 0.7).
		WithAttribute("decision.edge", 0.012).
		WithAttribute("decision.expected_return_bps", 25.0).
		WithAttribute("decision.economic_priced", true).
		WithAttribute("allowed", true)
	second := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("ETH/USD").
		WithAttribute("type", "market").
		WithAttribute("reason_source", "toxicity").
		WithAttribute("reason_category", "vacuum").
		WithAttribute("decision.confidence", 0.6).
		WithAttribute("decision.score", 0.4).
		WithAttribute("decision.edge", 0.008).
		WithAttribute("decision.expected_return_bps", 15.0).
		WithAttribute("decision.economic_priced", true).
		WithAttribute("allowed", true)

	crypto := &Crypto{audit: recorder}
	crypto.writeCandidateDecisionAudit(7, []verdict{
		{action: first, reason: "admitted"},
		{action: second, reason: "admitted"},
	}, nil)
	if err := recorder.Close(); err != nil {
		t.Fatalf("close audit recorder: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	defer file.Close()

	samples, err := optimizer.ReadJSONL(file)
	if err != nil {
		t.Fatalf("optimizer read audit jsonl: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	if samples[0].Source == samples[1].Source {
		t.Fatalf("expected separate candidate rows, got %#v", samples)
	}
}
