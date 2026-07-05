package cmd

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	dashboard "github.com/theapemachine/symm/ui"
)

type accountObserver interface {
	Observe(map[string]any) error
}

type accountPublisher interface {
	Publish(dashboard.Message) error
}

type accountBridge struct {
	ctx       context.Context
	cancel    context.CancelFunc
	source    websocket.PrivateAccount
	observer  accountObserver
	publisher accountPublisher
	frames    chan map[string]any
	interval  time.Duration
}

func newAccountBridge(
	ctx context.Context,
	source websocket.PrivateAccount,
	observer accountObserver,
	publisher accountPublisher,
	interval time.Duration,
) *accountBridge {
	ctx, cancel := context.WithCancel(ctx)

	return &accountBridge{
		ctx:       ctx,
		cancel:    cancel,
		source:    source,
		observer:  observer,
		publisher: publisher,
		frames:    source.Observe(),
		interval:  interval,
	}
}

func (bridge *accountBridge) Start() error {
	if bridge.interval <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"account: sync interval required",
			nil,
		))
	}

	go bridge.observe()

	if err := bridge.source.Sync(); err != nil {
		return err
	}

	go bridge.sync()
	return nil
}

func (bridge *accountBridge) observe() {
	for {
		select {
		case <-bridge.ctx.Done():
			return
		case frame := <-bridge.frames:
			if err := bridge.publish(frame); err != nil {
				errnie.Error(err)
				bridge.cancel()
				return
			}
		}
	}
}

func (bridge *accountBridge) sync() {
	ticker := time.NewTicker(bridge.interval)
	defer ticker.Stop()

	for {
		select {
		case <-bridge.ctx.Done():
			return
		case <-ticker.C:
			if err := bridge.source.Sync(); err != nil {
				errnie.Error(err)
				bridge.cancel()
				return
			}
		}
	}
}

func (bridge *accountBridge) publish(frame map[string]any) error {
	channel := bridge.text(frame["channel"])
	if channel == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"account: frame channel required",
			nil,
		))
	}

	rows, err := bridge.rows(frame)
	if err != nil {
		return err
	}

	count, err := bridge.count(frame, rows)
	if err != nil {
		return err
	}

	at := bridge.timestamp(frame)
	message, err := bridge.message(channel, rows, count, at)
	if err != nil {
		return err
	}

	if err := bridge.observer.Observe(frame); err != nil {
		return err
	}

	return bridge.publisher.Publish(message)
}

func (bridge *accountBridge) rows(frame map[string]any) ([]map[string]any, error) {
	switch data := frame["data"].(type) {
	case nil:
		return []map[string]any{}, nil
	case []map[string]any:
		return data, nil
	case []any:
		rows := make([]map[string]any, 0, len(data))
		for _, item := range data {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"account: data row object required",
					nil,
				))
			}

			rows = append(rows, row)
		}

		return rows, nil
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: data rows required",
			nil,
		))
	}
}

func (bridge *accountBridge) count(frame map[string]any, rows []map[string]any) (int, error) {
	count := bridge.text(frame["count"])
	if count == "" {
		return len(rows), nil
	}

	parsed, err := strconv.ParseFloat(count, 64)
	if err != nil || math.Trunc(parsed) != parsed {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: count must be an integer",
			err,
		))
	}

	return int(parsed), nil
}

func (bridge *accountBridge) message(
	channel string,
	rows []map[string]any,
	count int,
	at string,
) (dashboard.Message, error) {
	switch channel {
	case "balances":
		return dashboard.Message{
			Balances: &dashboard.Balances{Rows: rows, Count: count, At: at},
		}, nil
	case "orders":
		return dashboard.Message{
			Orders: &dashboard.Orders{Rows: rows, Count: count, At: at},
		}, nil
	case "executions":
		return dashboard.Message{
			Executions: &dashboard.Executions{Rows: rows, Count: count, At: at},
		}, nil
	default:
		return dashboard.Message{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: unsupported frame channel: "+channel,
			nil,
		))
	}
}

func (bridge *accountBridge) timestamp(frame map[string]any) string {
	observed := bridge.text(frame["timestamp"])
	if observed != "" {
		return observed
	}

	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (bridge *accountBridge) text(value any) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(value))
}

func (bridge *accountBridge) Close() error {
	bridge.cancel()
	return nil
}
