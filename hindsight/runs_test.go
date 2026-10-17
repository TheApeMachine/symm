package hindsight

import "testing"

func TestLastPresent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		last uint64
	}{
		{name: "empty", last: 0},
		{name: "one", last: 1},
		{name: "exact power of two", last: 8},
		{name: "just past a power of two", last: 9},
		{name: "collector sized", last: 168133},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := lastPresent(func(tick uint64) bool {
				return tick >= 1 && tick <= tc.last
			})

			if got != tc.last {
				t.Fatalf("lastPresent() = %d, want %d", got, tc.last)
			}
		})
	}
}
