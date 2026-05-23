package ui

import (
	"net/http/httptest"
	"testing"
)

func TestAllowLocalhostOrigin(t *testing.T) {
	request := httptest.NewRequest("GET", "http://127.0.0.1:8765/ws", nil)
	request.Header.Set("Origin", "http://localhost:5173")

	if !allowLocalhostOrigin(request) {
		t.Fatal("expected localhost origin to be allowed")
	}
}

func TestAllowLocalhostOriginRejectsRemote(t *testing.T) {
	request := httptest.NewRequest("GET", "http://127.0.0.1:8765/ws", nil)
	request.Header.Set("Origin", "https://evil.example")

	if allowLocalhostOrigin(request) {
		t.Fatal("expected remote origin to be rejected")
	}
}
