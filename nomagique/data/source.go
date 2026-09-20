package data

import (
	"context"
	"crypto/tls"
	"net/http"
	"strconv"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type SourceNode types.StreamNode[any, any]

type SourceImpl struct {
	Downstream func(context.Context, capnp.Ptr) error
	focusChan  chan string
	conn       *websocket.Conn
}

type WSMessage struct {
	Event        string   `json:"event,omitempty"`
	Pair         []string `json:"pair,omitempty"`
	Subscription struct {
		Name string `json:"name,omitempty"`
	} `json:"subscription,omitempty"`
}

func NewSource() SourceNode {
	impl := &SourceImpl{}

	return types.NewStreamNode(
		impl,
		func(ctx context.Context, payload any) error {
			if ch, ok := ctx.Value("focusChan").(chan string); ok {
				impl.focusChan = ch
			}
			// Trigger initialization of the websocket when the pipeline starts
			go impl.startKrakenFeed(ctx)
			return nil
		},
		func(next func(context.Context, any) error) {
			impl.Downstream = func(c context.Context, ptr capnp.Ptr) error {
				return next(c, ptr)
			}
		},
	)
}

func (s *SourceImpl) startKrakenFeed(ctx context.Context) {
	url := "wss://ws.kraken.com"
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[source] failed to connect to kraken", err))
		return
	}
	s.conn = conn

	errnie.Info("[source] connected to Kraken WS")

	go s.readLoop(ctx)
	go s.subscribeLoop(ctx)
}

func (s *SourceImpl) readLoop(ctx context.Context) {
	defer s.conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, msg, err := s.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					errnie.Error(errnie.Err(errnie.IO, "[source] unexpected close error", err))
				} else {
					errnie.Warn("[source] connection closed")
				}
				// Attempt to reconnect after a delay, or just return for now
				return
			}

			if len(msg) > 0 && msg[0] == '[' {
				var raw []any
				if err := sonic.Unmarshal(msg, &raw); err == nil && len(raw) >= 4 {
					if name, ok := raw[2].(string); ok && name == "trade" {
						pair, _ := raw[3].(string)
						trades, _ := raw[1].([]any)

						for _, t := range trades {
							if trade, ok := t.([]any); ok && len(trade) >= 3 {
								priceStr, _ := trade[0].(string)
								price, _ := strconv.ParseFloat(priceStr, 64)

								_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
								if err == nil {
									m, _ := NewRootWireMeasurement(seg)
									m.SetId([]byte(pair))
									m.SetEpoch(time.Now().UnixNano())
									m.SetSource(WireMeasurement_SourceType_public)
									m.SetEntity(WireMeasurement_EntityType_trade)
									m.SetLabel([]byte("trade"))

									mList, _ := m.NewMetrics(1)
									met := mList.At(0)
									met.SetRaw(price)

									if s.Downstream != nil {
										_ = s.Downstream(ctx, m.ToPtr())
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func (s *SourceImpl) subscribeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case symbol := <-s.focusChan:
			errnie.Info("[source] Subscribing to " + symbol)
			sub := WSMessage{
				Event: "subscribe",
				Pair:  []string{symbol},
			}
			sub.Subscription.Name = "trade"
			b, _ := sonic.Marshal(sub)
			if s.conn != nil {
				_ = s.conn.WriteMessage(websocket.TextMessage, b)
			}
		}
	}
}
