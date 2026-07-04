package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/websocket"
)

type accountBridge struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	source      websocket.PrivateAccount
	frames      chan map[string]any
	ui          *qpool.BroadcastGroup
	private     *qpool.BroadcastGroup
	roles       map[string]*qpool.BroadcastGroup
	subscribers []*qpool.BroadcastConsumer
	interval    time.Duration
}

func newAccountBridge(
	ctx context.Context,
	pool *qpool.Q[any],
	source websocket.PrivateAccount,
	interval time.Duration,
) *accountBridge {
	ctx, cancel := context.WithCancel(ctx)

	bridge := &accountBridge{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		source:   source,
		frames:   source.Observe(),
		ui:       pool.CreateBroadcastGroup("ui"),
		private:  pool.CreateBroadcastGroup("kraken:private"),
		roles:    map[string]*qpool.BroadcastGroup{},
		interval: interval,
	}

	for _, role := range []string{"balances", "orders", "executions"} {
		bridge.roles[role] = pool.CreateBroadcastGroup(role)
	}

	bridge.subscribers = append(
		bridge.subscribers,
		pool.Subscribe("kraken:private", bridge.submit),
	)

	return bridge
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
	artifact, err := bridge.artifact(frame)
	if err != nil {
		return err
	}

	role, err := artifact.Role()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "account: frame role", err))
	}

	group := bridge.roles[role]
	if group == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"account: unsupported frame channel: "+role,
			nil,
		))
	}

	if err := group.Send(artifact); err != nil {
		return err
	}

	return bridge.ui.Send(artifact)
}

func (bridge *accountBridge) artifact(frame map[string]any) (*datura.Artifact, error) {
	if len(frame) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: empty frame",
			nil,
		))
	}

	role := bridge.text(frame["channel"])
	if role == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: frame channel required",
			nil,
		))
	}

	artifact := datura.Acquire("kraken:account", datura.APPJSON).
		WithDestination("broker").
		WithRole(role).
		WithScope(role).
		WithPayload(datura.Map[any](frame).Marshal())

	if observed := bridge.text(frame["timestamp"]); observed != "" {
		at, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"account: frame timestamp",
				err,
			))
		}

		artifact.SetTimestamp(at.UnixNano())
	}

	return artifact, nil
}

func (bridge *accountBridge) text(value any) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(value))
}

func (bridge *accountBridge) submit(artifact *datura.Artifact) error {
	return bridge.source.Submit(artifact)
}

func (bridge *accountBridge) Close() error {
	bridge.cancel()
	return nil
}
