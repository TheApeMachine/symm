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
