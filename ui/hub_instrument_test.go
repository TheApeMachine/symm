package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wswebsocket "github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/trader"
)

func TestHubInstrumentPathPublishesPumpDumpMeasurement(testingTB *testing.T) {
	Convey("Given a dashboard websocket and Kraken instrument stream", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		upgrader := wswebsocket.Upgrader{
			CheckOrigin: func(request *http.Request) bool {
				return true
			},
		}

		kraken := httptest.NewServer(http.HandlerFunc(
			func(response http.ResponseWriter, request *http.Request) {
				conn, err := upgrader.Upgrade(response, request, nil)

				if err != nil {
					testingTB.Errorf("kraken upgrade failed: %v", err)
					return
				}

				defer conn.Close()

				for {
					_, wire, err := conn.ReadMessage()

					if err != nil {
						return
					}

					var frame struct {
						Params struct {
							Channel string `json:"channel"`
						} `json:"params"`
					}

					if json.Unmarshal(wire, &frame) != nil {
						continue
					}

					switch frame.Params.Channel {
					case "instrument":
						_ = conn.WriteMessage(wswebsocket.TextMessage, []byte(
							`{"channel":"instrument","type":"snapshot","data":{"pairs":[{"symbol":"ETH/USD","quote":"USD"}]}}`,
						))
					case "ticker":
						sendTickerFrames(testingTB, conn)
					}
				}
			},
		))
		defer kraken.Close()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)

		listenAddr := listener.Addr().String()
		So(listener.Close(), ShouldBeNil)

		previousAddr := viper.GetString("ui.addr")
		previousQuote := viper.GetString("market.quote_currency")
		previousLimit := viper.GetInt("market.max_scan_symbols")
		previousInterval := viper.Get("market.story.ui_interval")
		previousDelay := viper.GetInt("system.network.connection.max_delay")

		viper.Set("ui.addr", listenAddr)
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.max_scan_symbols", 1)
		viper.Set("market.story.ui_interval", 20*time.Millisecond)
		viper.Set("system.network.connection.max_delay", 3)

		defer func() {
			viper.Set("ui.addr", previousAddr)
			viper.Set("market.quote_currency", previousQuote)
			viper.Set("market.max_scan_symbols", previousLimit)
			viper.Set("market.story.ui_interval", previousInterval)
			viper.Set("system.network.connection.max_delay", previousDelay)
		}()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		tree := dmt.NewTree("")
		publicSocket := public.NewWebSocket(ctx, pool, tree)
		crypto := trader.NewCrypto(ctx, pool, tree)
		hub := NewHub(ctx, pool)
		serverErrors := make(chan error, 1)

		go publicSocket.Run(public.EndpointType(
			"ws" + strings.TrimPrefix(kraken.URL, "http"),
		))
		go func() {
			serverErrors <- hub.Run()
		}()
		go func() {
			_ = crypto.Run()
		}()

		defer func() {
			_ = hub.Close()
			_ = crypto.Close()
			_ = publicSocket.Close()

			select {
			case err := <-serverErrors:
				if err != nil && !strings.Contains(err.Error(), "server is not running") {
					testingTB.Errorf("hub run failed: %v", err)
				}
			case <-time.After(time.Second):
				testingTB.Errorf("hub did not stop")
			}
		}()

		var conn *wswebsocket.Conn

		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			conn, _, err = wswebsocket.DefaultDialer.Dial("ws://"+listenAddr+"/ws", nil)

			if err == nil {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		So(err, ShouldBeNil)
		defer conn.Close()
		waitHubClient(testingTB, hub)

		Convey("It should deliver a pumpdump measurement and decision trace", func() {
			So(readPumpDumpMeasurement(conn), ShouldBeTrue)
			So(readDecisionTrace(conn), ShouldBeTrue)
		})
	})
}

func sendTickerFrames(testingTB testing.TB, conn *wswebsocket.Conn) {
	testingTB.Helper()

	base := time.Date(2026, 6, 23, 8, 0, 0, 0, time.UTC)

	for tick := range 80 {
		last := 1000 + float64(tick)*0.4
		volume := 1000 + float64(tick*tick+1)
		payload := fmt.Sprintf(
			`{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","bid":%g,"ask":%g,"last":%g,"volume":%g,"timestamp":%q}]}`,
			last-0.1,
			last+0.1,
			last,
			volume,
			base.Add(time.Duration(tick)*time.Second).Format(time.RFC3339Nano),
		)

		if err := conn.WriteMessage(wswebsocket.TextMessage, []byte(payload)); err != nil {
			testingTB.Errorf("ticker write failed: %v", err)
			return
		}
	}
}

func readPumpDumpMeasurement(conn *wswebsocket.Conn) bool {
	deadline := time.Now().Add(5 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		_, wire, err := conn.ReadMessage()

		if err != nil {
			return false
		}

		var artifact datura.Artifact

		if _, err := artifact.Unpack(wire); err != nil {
			continue
		}

		origin, _ := artifact.Origin()
		role, _ := artifact.Role()

		if origin == "pumpdump" && role == "measurement" {
			return true
		}
	}

	return false
}

func readDecisionTrace(conn *wswebsocket.Conn) bool {
	deadline := time.Now().Add(5 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		_, wire, err := conn.ReadMessage()

		if err != nil {
			return false
		}

		var artifact datura.Artifact

		if _, err := artifact.Unpack(wire); err != nil {
			continue
		}

		var frame struct {
			Type      string `json:"type"`
			Decisions []struct {
				Symbol string `json:"symbol"`
			} `json:"decisions"`
		}

		if json.Unmarshal(artifact.DecryptPayload(), &frame) != nil {
			continue
		}

		if frame.Type == "decision_trace" && len(frame.Decisions) > 0 {
			return true
		}
	}

	return false
}
