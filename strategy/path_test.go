package strategy

import (
	"testing"

	"github.com/theapemachine/symm/strategy/tokenizer"
)

func TestPathExitPrefixAfterReset(t *testing.T) {
	path := NewPath()
	symbol := "BTC/USD"

	path.Append(symbol, tokenizer.NewToken([]string{"R01"}, 1.2, 0))
	path.Append(symbol, tokenizer.NewToken([]string{"R02"}, 1.1, 0))

	if got := path.Live(symbol); got != "R01/R02" {
		t.Fatalf("enter live = %q", got)
	}

	path.Reset(symbol)

	path.Append(symbol, tokenizer.NewToken([]string{"R04"}, 1.3, 0))

	live := path.Live(symbol)
	if live != "R04" {
		t.Fatalf("exit live = %q, want separate root R04", live)
	}
}

func TestPathExitTriggerPrefix(t *testing.T) {
	path := NewPath()
	symbol := "BTC/USD"

	path.Append(symbol, tokenizer.NewToken([]string{"R06"}, 1.0, 0))
	path.AppendTrigger(symbol, tokenizer.NewToken([]string{"R03"}, 1.2, 0))
	path.AppendTrigger(symbol, tokenizer.NewToken([]string{"R04"}, 1.1, 0))

	if got := path.TriggerLive(symbol); got != "R03/R04" {
		t.Fatalf("trigger = %q, want R03/R04", got)
	}

	if !path.HasTrigger(symbol) {
		t.Fatal("expected trigger path")
	}

	if got := path.PolicyLive(symbol, false); got != "R03/R04" {
		t.Fatalf("policy live hold = %q", got)
	}
}
