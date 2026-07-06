package websocket

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type PaperPrivate struct {
	ctx        context.Context
	cancel     context.CancelFunc
	stream     *Stream
	paper      *kraken.PaperCLI
	executions map[string]bool
}

func NewPaperPrivate(ctx context.Context) *PaperPrivate {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetViper().GetInt("system.websocket.channel.buffer")

	if buffer < 1 {
		buffer = 1
	}

	return &PaperPrivate{
		ctx:        ctx,
		cancel:     cancel,
		stream:     NewStream(buffer),
		paper:      kraken.NewPaperCLI(),
		executions: map[string]bool{},
	}
}

func (private *PaperPrivate) Observe(channel string) chan []byte {
	observer := private.stream.Observe(channel)

	switch channel {
	case "balances":
		errnie.Error(private.publishBalances())
	case "executions":
		errnie.Error(private.rememberExecutions())
	}

	return observer
}

func (private *PaperPrivate) Submit(order *kraken.Order) error {
	if err := private.paper.Submit(private.ctx, order); err != nil {
		return err
	}

	if err := private.publishBalances(); err != nil {
		return err
	}

	return private.publishExecutions()
}

func (private *PaperPrivate) Close() {
	private.cancel()
}

func (private *PaperPrivate) publishBalances() error {
	rows, err := private.paper.Balances(private.ctx)

	if err != nil {
		return err
	}

	buf, err := sonic.Marshal(rows)

	if err != nil {
		return err
	}

	private.stream.Receive(append([]byte(`{"channel":"balances","data":`), append(buf, '}')...))
	return nil
}

func (private *PaperPrivate) publishExecutions() error {
	rows, err := private.paper.Executions(private.ctx)

	if err != nil {
		return err
	}

	updates := make([]kraken.ExecutionData, 0, len(rows))

	for _, row := range rows {
		if private.executions[row.ExecID] {
			continue
		}

		private.executions[row.ExecID] = true
		updates = append(updates, row)
	}

	if len(updates) == 0 {
		return nil
	}

	buf, err := sonic.Marshal(updates)

	if err != nil {
		return err
	}

	private.stream.Receive(append([]byte(`{"channel":"executions","data":`), append(buf, '}')...))
	return nil
}

func (private *PaperPrivate) rememberExecutions() error {
	rows, err := private.paper.Executions(private.ctx)

	if err != nil {
		return err
	}

	for _, row := range rows {
		private.executions[row.ExecID] = true
	}

	return nil
}
