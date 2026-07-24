package types

import (
	"testing"
)

/*
FuzzStatusTransitionTable proves the transition table accepts only declared edges.
*/
func FuzzStatusTransitionTable(f *testing.F) {
	f.Add(string(PENDING), string(OPEN))
	f.Add(string(CLOSED), string(PENDING))

	f.Fuzz(func(t *testing.T, fromRaw, toRaw string) {
		from := Status(fromRaw)
		to := Status(toRaw)

		next, err := Transition(from, to)

		if from == to {
			if err != nil {
				t.Fatalf("idempotent transition failed: %v", err)
			}

			if next != to {
				t.Fatalf("idempotent transition changed status: got %q want %q", next, to)
			}

			return
		}

		if to == Status("cancelled") {
			to = CANCELED
		}

		allowed, ok := statusEdges[from]

		if !ok {
			if err == nil {
				t.Fatalf("unknown source accepted transition to %q", to)
			}

			return
		}

		_, edgeExists := allowed[to]

		if edgeExists {
			if err != nil {
				t.Fatalf("legal transition rejected: %s -> %s: %v", from, to, err)
			}

			if next != to {
				t.Fatalf("transition returned %q, want %q", next, to)
			}

			return
		}

		if err == nil {
			t.Fatalf("illegal transition accepted: %s -> %s", from, to)
		}
	})
}
