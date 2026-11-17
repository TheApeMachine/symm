package network

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestNewREST(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewREST is called", func() {
			rest := NewREST()

			Convey("the client is wired", func() {
				So(rest, ShouldNotBeNil)
				So(rest.conn, ShouldNotBeNil)
			})
		})
	})
}

func TestPost(t *testing.T) {
	Convey("Setup", t, func() {
		var signed bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("API-Key") != "" || r.Header.Get("API-Sign") != "" {
				signed = true
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		Convey("When Post hits a public Kraken path", func() {
			_, err := NewREST().Post(server.URL+"/0/public/Time", map[string]string{"pair": "XBTUSD"})

			Convey("the request is unsigned", func() {
				So(err, ShouldBeNil)
				So(signed, ShouldBeFalse)
			})
		})

		Convey("When Post targets a private path on another host", func() {
			_, err := NewREST().Post("https://example.com/0/private/AddOrder", nil)

			Convey("it fails", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestGet(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Get targets a private Kraken path", func() {
			_, err := NewREST().Get("https://api.kraken.com/0/private/Balance", nil)

			Convey("validation rejects private GET", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[network.rest] private endpoints are POST")
			})
		})
	})
}

func TestRestTransient(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When fasthttp reports an early server close", func() {
			err := errnie.Err(
				errnie.IO,
				"fasthttp: the server closed connection before returning the first response byte",
				nil,
			)

			Convey("restTransient and Recoverable agree", func() {
				So(restTransient(err), ShouldBeTrue)
				So(Recoverable(err), ShouldBeTrue)
			})
		})

		Convey("When the error is a logic bug", func() {
			err := errnie.Err(errnie.Validation, "bad args", nil)

			Convey("restTransient is false", func() {
				So(restTransient(err), ShouldBeFalse)
			})
		})
	})
}

func TestPostRetriesTransientClose(t *testing.T) {
	Convey("Setup", t, func() {
		var hits atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				hj, ok := w.(http.Hijacker)
				if !ok {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				conn, _, err := hj.Hijack()
				if err != nil {
					return
				}

				_ = conn.Close()
				return
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"error":[]}`))
		}))
		defer server.Close()

		Convey("When the first dial is closed before a response", func() {
			_, err := NewREST().Post(server.URL+"/0/public/Time", map[string]string{})

			Convey("the second attempt succeeds", func() {
				So(err, ShouldBeNil)
				So(hits.Load(), ShouldEqual, 2)
			})
		})
	})
}

func TestKrakenSignPrivate(t *testing.T) {
	Convey("Setup", t, func() {
		secret := base64.StdEncoding.EncodeToString([]byte("secret"))
		t.Setenv("TEST_KRAKEN_KEY", "api-key")
		t.Setenv("TEST_KRAKEN_SECRET", secret)
		viper.Set("kraken.credentials.main.key_env", "TEST_KRAKEN_KEY")
		viper.Set("kraken.credentials.main.secret_env", "TEST_KRAKEN_SECRET")
		viper.Set("system.data_path", t.TempDir())

		So(kraken.UseCredentials(kraken.RoleMain), ShouldBeNil)

		Convey("When SignPrivate signs an AddOrder body", func() {
			key, signature, signed, err := kraken.SignPrivate("/0/private/AddOrder", map[string]string{
				"pair":      "XBTUSD",
				"type":      "buy",
				"ordertype": "market",
				"volume":    "0.1",
			})

			Convey("signature matches spot.Sign", func() {
				So(err, ShouldBeNil)
				So(key, ShouldEqual, "api-key")

				var payload map[string]any
				So(sonic.Unmarshal(signed, &payload), ShouldBeNil)
				nonce, _ := payload["nonce"].(string)
				So(nonce, ShouldNotBeEmpty)
				So(payload["pair"], ShouldEqual, "XBTUSD")

				expect, err := spot.Sign(secret, "/0/private/AddOrder", nonce, io.NopCloser(bytes.NewReader(signed)))
				So(err, ShouldBeNil)
				So(signature, ShouldEqual, expect)
			})
		})
	})
}

func BenchmarkNewREST(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewREST()
	}
}

func BenchmarkPost(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	rest := NewREST()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = rest.Post(server.URL+"/0/public/Time", map[string]string{"pair": "XBTUSD"})
	}
}

func BenchmarkGet(b *testing.B) {
	rest := NewREST()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = rest.Get("https://api.kraken.com/0/private/Balance", nil)
	}
}

func setupKrakenSignBench(b *testing.B) {
	b.Helper()
	secret := base64.StdEncoding.EncodeToString([]byte("secret"))
	b.Setenv("TEST_KRAKEN_KEY", "api-key")
	b.Setenv("TEST_KRAKEN_SECRET", secret)
	viper.Set("kraken.credentials.main.key_env", "TEST_KRAKEN_KEY")
	viper.Set("kraken.credentials.main.secret_env", "TEST_KRAKEN_SECRET")
	viper.Set("system.data_path", b.TempDir())
	if err := kraken.UseCredentials(kraken.RoleMain); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkKrakenSignPrivate(b *testing.B) {
	setupKrakenSignBench(b)
	body := map[string]string{
		"pair": "XBTUSD", "type": "buy", "ordertype": "market", "volume": "0.1",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _, _, _ = kraken.SignPrivate("/0/private/AddOrder", body)
	}
}
