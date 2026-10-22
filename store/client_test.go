package store

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestClientRefusesEmptyUploadKey(t *testing.T) {
	client := &Client{
		System: runtime.NewSystem(context.Background(), "store.client"),
		bucket: "test-bucket",
	}

	n, err := client.WriteAtPrefix("", []byte("payload"))
	if err == nil {
		t.Fatal("expected error for empty upload key")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}

	n, err = client.Write([]byte("payload"))
	if err == nil {
		t.Fatal("expected error for Write without prefix")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}
}

func TestDestKeyKeepsPathUnderSourcePrefix(t *testing.T) {
	cases := []struct {
		name   string
		source string
		dest   string
		key    string
		want   string
	}{
		{
			name:   "path under the source prefix is kept",
			source: "position/open/BTC/USD",
			dest:   "position/closed/BTC/USD",
			key:    "position/open/BTC/USD/1792668320433768000/0000000000000000230",
			want:   "position/closed/BTC/USD/1792668320433768000/0000000000000000230",
		},
		{
			name:   "a trailing separator on the source prefix",
			source: "position/open/BTC/USD/",
			dest:   "position/closed/BTC/USD/",
			key:    "position/open/BTC/USD/1792668320433768000/0000000000000000230",
			want:   "position/closed/BTC/USD/1792668320433768000/0000000000000000230",
		},
		{
			name:   "the object is the prefix itself",
			source: "position/open/BTC/USD/1792668320433768000/0000000000000000230",
			dest:   "position/closed/BTC/USD/1792668320433768000/0000000000000000230",
			key:    "position/open/BTC/USD/1792668320433768000/0000000000000000230",
			want:   "position/closed/BTC/USD/1792668320433768000/0000000000000000230",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := destKey(test.dest, test.key, test.source)

			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}

			if got == test.key {
				t.Fatal("destination key copies the object onto itself")
			}
		})
	}
}
