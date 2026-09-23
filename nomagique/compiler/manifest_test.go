package compiler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/manifest"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Every manifest that ships must compile, so a graph referencing a primitive
that does not exist is caught here rather than at boot.
*/
func TestManifestsCompile(t *testing.T) {
	Convey("Given the manifests that ship with the system", t, func() {
		identifiers, err := manifest.List()
		So(err, ShouldBeNil)
		So(len(identifiers), ShouldBeGreaterThan, 0)

		for _, identifier := range identifiers {
			Convey("It compiles "+identifier, func() {
				graph, err := DefaultRepository().Load(identifier)
				So(err, ShouldBeNil)

				program, err := Compile(graph, nil, DefaultRepository())
				So(err, ShouldBeNil)

				// A graph describes work to do or a surface to show it on, so
				// one that lowered to neither did not describe anything.
				surfaces := 0

				if program.UI != nil {
					surfaces = len(program.UI.Routes)
				}

				So(len(program.Nodes)+surfaces, ShouldBeGreaterThan, 0)
			})
		}
	})
}

/*
fakeKraken plays the exchange for manifest/live_level3.json: a token endpoint
that only answers a correctly signed request, and a level 3 socket that opens
with a status frame, beats, rate limits one symbol once and records every
subscription it accepts.
*/
type fakeKraken struct {
	mu         sync.Mutex
	accepted   []string
	tokens     []string
	limited    bool
	signatures int
}

func (venue *fakeKraken) tokenServer(key string, secret []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)

		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		nonce := strings.TrimPrefix(string(body), "nonce=")
		digest := sha256.Sum256([]byte(nonce + string(body)))
		mac := hmac.New(sha512.New, secret)
		mac.Write(append([]byte(request.URL.Path), digest[:]...))
		expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		if request.Header.Get("API-Key") != key || request.Header.Get("API-Sign") != expected ||
			request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			fmt.Fprint(writer, `{"error":["EAPI:Invalid signature"]}`)
			return
		}
		venue.mu.Lock()
		venue.signatures++
		venue.mu.Unlock()
		fmt.Fprint(writer, `{"error":[],"result":{"token":"issued-token","expires":900}}`)
	}))
}

func (venue *fakeKraken) socketServer() *httptest.Server {
	upgrader := gorillaws.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)

		if err != nil {
			return
		}
		defer connection.Close()
		var write sync.Mutex
		send := func(frame string) error {
			write.Lock()
			defer write.Unlock()
			return connection.WriteMessage(gorillaws.TextMessage, []byte(frame))
		}

		if send(`{"channel":"status","type":"update","data":[{"system":"online","api_version":"v2"}]}`) != nil {
			return
		}
		done := make(chan struct{})
		defer close(done)
		subscribed := make(chan struct{})
		var beating sync.Once

		// Like the exchange, the socket only beats once something is subscribed.
		go func() {
			select {
			case <-done:
				return
			case <-subscribed:
			}
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if send(`{"channel":"heartbeat"}`) != nil {
						return
					}
				}
			}
		}()

		for {
			_, payload, err := connection.ReadMessage()

			if err != nil {
				return
			}
			var message struct {
				Params struct {
					Symbol []string `json:"symbol"`
					Token  string   `json:"token"`
					Depth  int      `json:"depth"`
				} `json:"params"`
			}

			if json.Unmarshal(payload, &message) != nil || len(message.Params.Symbol) != 1 {
				return
			}
			symbol := message.Params.Symbol[0]
			beating.Do(func() { close(subscribed) })
			venue.mu.Lock()
			limit := symbol == "RATE/USD" && !venue.limited
			venue.limited = venue.limited || limit

			if !limit {
				venue.accepted = append(venue.accepted, symbol)
				venue.tokens = append(venue.tokens, message.Params.Token)
			}
			venue.mu.Unlock()

			if limit {
				send(`{"method":"subscribe","success":false,"symbol":"RATE/USD","error":"Rate limit for snapshot requests exceeded"}`)
				continue
			}
			send(`{"method":"subscribe","success":true,"result":{"channel":"level3","symbol":"` + symbol + `"}}`)
		}
	}))
}

func TestLiveLevel3Manifest(t *testing.T) {
	Convey("Given the level 3 ingress graph against a fake exchange", t, func() {
		secret := []byte("level three secret")
		t.Setenv("L3_API_KEY", "level-three-key")
		t.Setenv("L3_API_SECRET", base64.StdEncoding.EncodeToString(secret))
		venue := &fakeKraken{}
		tokens := venue.tokenServer("level-three-key", secret)
		defer tokens.Close()
		sockets := venue.socketServer()
		defer sockets.Close()

		raw, err := manifest.ReadFile("live_level3")
		So(err, ShouldBeNil)
		document := strings.NewReplacer(
			"wss://ws-l3.kraken.com/v2", "ws"+strings.TrimPrefix(sockets.URL, "http"),
			"https://api.kraken.com", tokens.URL,
		).Replace(string(raw))
		program, err := CompileJSON([]byte(document), nil, NewRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		symbols, err := transport.NewFan_write_Params(segment)
		So(err, ShouldBeNil)
		So(symbols.SetData([]byte(`["BTC/USD","RATE/USD","ETH/USD"]`)), ShouldBeNil)
		So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["symbols"]: capnp.Struct(symbols)}), ShouldBeNil)

		deadline := time.Now().Add(10 * time.Second)

		for time.Now().Before(deadline) {
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			venue.mu.Lock()
			done := len(venue.accepted) == 3
			venue.mu.Unlock()

			if done {
				break
			}
			time.Sleep(time.Millisecond)
		}

		Convey("Then every symbol is subscribed once, with the token its connection was issued", func() {
			venue.mu.Lock()
			defer venue.mu.Unlock()
			So(venue.signatures, ShouldEqual, 1)
			So(venue.accepted, ShouldResemble, []string{"BTC/USD", "ETH/USD", "RATE/USD"})
			So(venue.tokens, ShouldResemble, []string{"issued-token", "issued-token", "issued-token"})
		})
	})
}
