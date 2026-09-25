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
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/manifest"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
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
	mu                sync.Mutex
	accepted          []string
	tokens            []string
	limited           bool
	signatures        int
	connections       int
	connectionSymbols map[int][]string
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

		venue.mu.Lock()
		connID := venue.connections
		venue.connections++
		if venue.connectionSymbols == nil {
			venue.connectionSymbols = make(map[int][]string)
		}
		venue.mu.Unlock()

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
				venue.connectionSymbols[connID] = append(venue.connectionSymbols[connID], symbol)
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

		shardRaw, err := manifest.ReadFile("live_level3_shard")
		So(err, ShouldBeNil)
		shardDoc := strings.NewReplacer(
			"wss://ws-l3.kraken.com/v2", "ws"+strings.TrimPrefix(sockets.URL, "http"),
			"https://api.kraken.com", tokens.URL,
		).Replace(string(shardRaw))

		repo := NewRepository()
		So(repo.Save("live_level3_shard", []byte(shardDoc)), ShouldBeNil)

		program, err := CompileJSON([]byte(document), nil, repo)
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

func TestLiveLevel3Manifest_Sharded451(t *testing.T) {
	Convey("Given 451 online USD symbols across the sharded level 3 ingress graph", t, func() {
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

		shardRaw, err := manifest.ReadFile("live_level3_shard")
		So(err, ShouldBeNil)
		shardDoc := strings.NewReplacer(
			"wss://ws-l3.kraken.com/v2", "ws"+strings.TrimPrefix(sockets.URL, "http"),
			"https://api.kraken.com", tokens.URL,
		).Replace(string(shardRaw))

		repo := NewRepository()
		So(repo.Save("live_level3_shard", []byte(shardDoc)), ShouldBeNil)

		program, err := CompileJSON([]byte(document), nil, repo)
		So(err, ShouldBeNil)
		defer program.Release()

		// Generate 451 distinct symbols with RATE/USD in shard 0
		symbolList := make([]string, 451)
		for i := 0; i < 451; i++ {
			if i == 50 {
				symbolList[i] = "RATE/USD"
			} else {
				symbolList[i] = fmt.Sprintf("SYM%03d/USD", i)
			}
		}

		symbolsJSON, err := json.Marshal(symbolList)
		So(err, ShouldBeNil)

		_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		So(err, ShouldBeNil)
		symbols, err := transport.NewFan_write_Params(segment)
		So(err, ShouldBeNil)
		So(symbols.SetData(symbolsJSON), ShouldBeNil)
		So(program.Execute(context.Background(), map[NodeID]capnp.Struct{program.NodeMap["symbols"]: capnp.Struct(symbols)}), ShouldBeNil)

		deadline := time.Now().Add(10 * time.Second)

		for time.Now().Before(deadline) {
			So(program.Execute(context.Background(), nil), ShouldBeNil)
			venue.mu.Lock()
			done := len(venue.accepted) == 451
			venue.mu.Unlock()

			if done {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}

		Convey("Then exactly 3 physical connections are created with 200 / 200 / 51 membership", func() {
			venue.mu.Lock()
			defer venue.mu.Unlock()
			So(venue.connections, ShouldEqual, 3)
			lengths := []int{
				len(venue.connectionSymbols[0]),
				len(venue.connectionSymbols[1]),
				len(venue.connectionSymbols[2]),
			}
			sort.Ints(lengths)
			So(lengths, ShouldResemble, []int{51, 200, 200})
			So(len(venue.accepted), ShouldEqual, 451)
			So(venue.limited, ShouldBeTrue)

			seen := make(map[string]bool)
			for _, s := range venue.accepted {
				So(seen[s], ShouldBeFalse)
				seen[s] = true
			}
			So(len(seen), ShouldEqual, 451)
		})
	})
}

/*
TestImpulseMapManifest drives manifest/impulse_map.json with an impulse map of
six coordinates on a 3x2 lattice, interleaved as two communities: 0, 2 and 4
react together (4 consistently inverse) and 1, 3 and 5 react together, each
community to its own driver.
*/
func TestImpulseMapManifest(t *testing.T) {
	Convey("Given the impulse map fed two interleaved communities of coordinates", t, func() {
		var graph Graph
		So(json.Unmarshal([]byte(`{"id":"impulse_map_fixture","nodes":{
"feed":{"id":"feed","type":"store.Vector","inputData":{"width":{"value":1}},
 "connections":{"inputs":{},"outputs":{"values":[{"nodeId":"map","portName":"change.value"}],"found":[{"nodeId":"map","portName":"change.present"}]}}},
"map":{"id":"map","type":"definition:impulse_map",
 "connections":{"inputs":{"change.value":[{"nodeId":"feed","portName":"values"}],"change.present":[{"nodeId":"feed","portName":"found"}]},"outputs":{}}}
}}`), &graph), ShouldBeNil)

		program, err := Compile(graph, nil, DefaultRepository())
		So(err, ShouldBeNil)
		defer program.Release()

		ctx := context.Background()
		feed := store.Vector(program.Nodes[program.NodeMap["feed"]].Client)
		levels := make([]float64, 6)
		random := rand.New(rand.NewSource(7))
		community := []int{0, 1, 0, 1, 0, 1}
		direction := []float64{1, 1, 1, 1, -1, 1}

		// pass moves each community by its own driver, a member by the same
		// amount (scaled per coordinate), and runs the graph once.
		pass := func(drivers [2]float64) {
			for coordinate := range levels {
				levels[coordinate] += direction[coordinate] * float64(coordinate+1) * drivers[community[coordinate]]
			}

			So(feed.Write(ctx, func(params store.Vector_write_Params) error {
				params.SetWidth(1)
				index, err := params.NewIndex(6)

				if err != nil {
					return err
				}

				values, err := params.NewValues(6)

				if err != nil {
					return err
				}

				for coordinate, level := range levels {
					index.Set(coordinate, int64(coordinate))
					values.Set(coordinate, level)
				}

				return nil
			}), ShouldBeNil)
			So(feed.WaitStreaming(), ShouldBeNil)
			So(program.Execute(ctx, nil), ShouldBeNil)
		}

		driver := func() float64 {
			return math.Copysign(0.5+random.Float64(), random.Float64()-0.5)
		}

		for range 300 {
			pass([2]float64{driver(), driver()})
		}

		relaxed, found := program.Result("map__relaxation")
		So(found, ShouldBeTrue)
		positions, err := geometry.Relaxation_done_Results(relaxed).Positions()
		So(err, ShouldBeNil)
		So(positions.Len(), ShouldEqual, 12)

		distance := func(left, right int) float64 {
			return math.Hypot(
				positions.At(left*2)-positions.At(right*2),
				positions.At(left*2+1)-positions.At(right*2+1),
			)
		}

		Convey("Then each community gathers and the two drift apart", func() {
			within, across := 0.0, 0.0
			withinPairs, acrossPairs := 0, 0

			for left := range 6 {
				for right := left + 1; right < 6; right++ {
					if community[left] == community[right] {
						within += distance(left, right)
						withinPairs++
						continue
					}

					across += distance(left, right)
					acrossPairs++
				}
			}

			So(within/float64(withinPairs), ShouldBeLessThan, across/float64(acrossPairs))

			Convey("And the consistently inverse coordinate is gathered with its community", func() {
				So(distance(0, 4), ShouldBeLessThan, distance(0, 1))
			})
		})

		Convey("Then when only one community moves, only its regions light up", func() {
			pass([2]float64{driver(), 0})

			watershed, found := program.Result("map__peak")
			So(found, ShouldBeTrue)
			So(geometry.Watershed(watershed).Which(), ShouldEqual, geometry.Watershed_Which_settled)

			settled, err := geometry.Watershed(watershed).Settled().Regions()
			So(err, ShouldBeNil)

			moving := map[string]bool{}
			members := map[string]map[int]bool{}

			for coordinate := range 6 {
				region, err := settled.At(coordinate)
				So(err, ShouldBeNil)

				if members[region] == nil {
					members[region] = map[int]bool{}
				}

				members[region][community[coordinate]] = true

				if community[coordinate] == 0 {
					moving[region] = true
				}
			}

			// A region is one community's ground: the two never share one.
			for _, communities := range members {
				So(communities, ShouldHaveLength, 1)
			}

			result, found := program.Result("map__hot")
			So(found, ShouldBeTrue)
			hot, err := statistic.Otsu_done_Results(result).Hot()
			So(err, ShouldBeNil)
			So(hot.Len(), ShouldBeGreaterThan, 0)

			for position := range hot.Len() {
				token, err := hot.At(position)
				So(err, ShouldBeNil)
				So(moving[token], ShouldBeTrue)
			}
		})
	})
}
