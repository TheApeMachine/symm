package network

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

func TestRESTPublicPostIsUnsigned(t *testing.T) {
	var signed bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("API-Key") != "" || r.Header.Get("API-Sign") != "" {
			signed = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := NewREST().Post(server.URL+"/0/public/Time", map[string]string{"pair": "XBTUSD"}); err != nil {
		t.Fatalf("public post: %v", err)
	}
	if signed {
		t.Fatal("public call was signed")
	}

	if _, err := NewREST().Post("https://example.com/0/private/AddOrder", nil); err == nil {
		t.Fatal("expected a private url on another host to fail")
	}
	if _, err := NewREST().Get("https://api.kraken.com/0/private/Balance", nil); err == nil {
		t.Fatal("expected a private GET to fail")
	}
}

func TestRESTPrivateSignatureMatchesBody(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("secret"))
	t.Setenv("TEST_KRAKEN_KEY", "api-key")
	t.Setenv("TEST_KRAKEN_SECRET", secret)
	viper.Set("kraken.credentials.main.key_env", "TEST_KRAKEN_KEY")
	viper.Set("kraken.credentials.main.secret_env", "TEST_KRAKEN_SECRET")
	viper.Set("system.data_path", t.TempDir())

	if err := kraken.UseCredentials(kraken.RoleMain); err != nil {
		t.Fatalf("credentials: %v", err)
	}

	key, signature, signed, err := kraken.SignPrivate("/0/private/AddOrder", map[string]string{
		"pair":      "XBTUSD",
		"type":      "buy",
		"ordertype": "market",
		"volume":    "0.1",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if key != "api-key" {
		t.Fatalf("key %q", key)
	}

	var payload map[string]any
	if err := sonic.Unmarshal(signed, &payload); err != nil {
		t.Fatalf("body: %v", err)
	}
	nonce, _ := payload["nonce"].(string)
	if nonce == "" {
		t.Fatalf("body %s", signed)
	}
	if payload["pair"] != "XBTUSD" {
		t.Fatalf("pair %#v", payload["pair"])
	}

	expect, err := spot.Sign(secret, "/0/private/AddOrder", nonce, io.NopCloser(bytes.NewReader(signed)))
	if err != nil {
		t.Fatalf("spot sign: %v", err)
	}
	if signature != expect {
		t.Fatalf("signature %q", signature)
	}
}
