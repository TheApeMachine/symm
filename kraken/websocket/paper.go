package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	symmkraken "github.com/theapemachine/symm/kraken"
)

type paperFrame func(context.Context, *symmkraken.PaperCLI) ([]byte, error)

var paperChannels = map[string]paperFrame{
	"balances": func(ctx context.Context, cli *symmkraken.PaperCLI) ([]byte, error) {
		frame, err := cli.Balances(ctx)

		if err != nil {
			return nil, err
		}

		return sonic.Marshal(frame)
	},
	"orders": func(ctx context.Context, cli *symmkraken.PaperCLI) ([]byte, error) {
		frame, err := cli.Orders(ctx)

		if err != nil {
			return nil, err
		}

		return sonic.Marshal(frame)
	},
	"executions": func(ctx context.Context, cli *symmkraken.PaperCLI) ([]byte, error) {
		frame, err := cli.Executions(ctx)

		if err != nil {
			return nil, err
		}

		return sonic.Marshal(frame)
	},
}

var paperEndpoints = map[string]func(
	context.Context,
	*symmkraken.PaperCLI,
	json.Marshaler,
) ([]byte, error){
	symmkraken.TradeVolumeEndpoint: func(
		ctx context.Context,
		cli *symmkraken.PaperCLI,
		params json.Marshaler,
	) ([]byte, error) {
		return cli.Post(ctx, symmkraken.TradeVolumeEndpoint, params)
	},
}

/*
Paper is the simulated spot websocket and REST transport.
*/
type Paper struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	cli    *symmkraken.PaperCLI
	sync   *sync.Map
	url    string
	auth   bool
}

/*
NewPaper opens the paper spot transport.
*/
func NewPaper(
	ctx context.Context,
	pool *qpool.Q[any],
	baseURL string,
	auth bool,
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	return &Paper{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		cli:    symmkraken.NewPaperCLI(),
		sync:   &sync.Map{},
		url:    baseURL,
		auth:   auth,
	}
}

func (paper *Paper) On(
	channel string, action func([]byte),
) {
	callbacks, loaded := paper.sync.LoadOrStore(channel, []func([]byte){action})

	if loaded {
		stored := append(callbacks.([]func([]byte)), action)
		paper.sync.Store(channel, stored)
	}

	frame, ok := paperChannels[channel]

	if !ok {
		return
	}

	payload, err := frame(paper.ctx, paper.cli)

	if err != nil {
		errnie.Error(err)
		return
	}

	registered, _ := paper.sync.Load(channel)

	for _, callback := range registered.([]func([]byte)) {
		callback(payload)
	}
}

func (paper *Paper) Observe(channel string) chan []byte {
	outbound := make(chan []byte, 8)

	paper.On(channel, func(raw []byte) {
		select {
		case outbound <- raw:
		default:
			errnie.Error(errnie.Err(
				errnie.Conflict,
				"paper observe: channel full",
				nil,
			))
		}
	})

	return outbound
}

func (paper *Paper) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return err
	}

	order := symmkraken.Order{}

	if err := sonic.Unmarshal(raw, &order); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken: paper order decode failed",
			err,
		))
	}

	response, err := paper.cli.Submit(paper.ctx, &order)

	if err != nil {
		return err
	}

	payload, err := sonic.Marshal(response)

	if err != nil {
		return err
	}

	if callbacks, ok := paper.sync.Load(response.Method); ok {
		for _, callback := range callbacks.([]func([]byte)) {
			callback(payload)
		}
	}

	for channel, frame := range paperChannels {
		body, err := frame(paper.ctx, paper.cli)

		if err != nil {
			return err
		}

		if callbacks, ok := paper.sync.Load(channel); ok {
			for _, callback := range callbacks.([]func([]byte)) {
				callback(body)
			}
		}
	}

	return nil
}

func (paper *Paper) Get(
	path string, params json.Marshaler,
) ([]byte, error) {
	return nil, errnie.Error(errnie.Err(
		errnie.Validation,
		"paper get: not implemented",
		nil,
	))
}

func (paper *Paper) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	fetch, ok := paperEndpoints[path]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper post: unsupported path "+path,
			nil,
		))
	}

	return fetch(paper.ctx, paper.cli, params)
}

func (paper *Paper) Close() {
	paper.cancel()
}
