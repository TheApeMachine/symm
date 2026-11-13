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
	. "github.com/smartystreets/goconvey/convey"
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
