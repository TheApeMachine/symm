package private

import (
	"errors"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
)

func (ws *WebSocket) handleErrors(message *types.SocketMessage) {
	for _, errorText := range message.Errors {
		exchangeError := wsutil.ParseExchangeError(errorText)
		decision := wsutil.DefaultExchangeErrorPolicy().Classify(exchangeError)
		observability.Shared().RecordExchangeError(
			"kraken/private",
			exchangeError.Category,
			exchangeError.Code,
			string(decision.Action),
			exchangeError.Message,
			time.Now().UTC(),
		)

		handleErr := wsutil.HandleExchangePolicy(ws.ctx, exchangeError, decision)

		if handleErr == nil {
			continue
		}

		if internal.IsShutdown(handleErr) {
			return
		}

		errnie.Error(handleErr)
	}
}

func (ws *WebSocket) Close() error {
	var closeErr error

	if ws.conn != nil {
		closeErr = ws.conn.Close()

		if closeErr != nil {
			errnie.Error(closeErr)
		}
	}

	ws.isConnected.Store(false)
	ws.cancel()

	return closeErr
}

func (ws *WebSocket) subscribeBalances() error {
	if ws.conn == nil {
		return errors.New("private: websocket is not connected")
	}

	token, err := types.NewToken(ws.ctx)

	if err != nil {
		return err
	}

	return ws.conn.WriteJSON(user.SubscribeFrame{
		Method: "subscribe",
		Params: user.BalanceParams{
			Channel:  public.BalancesChannel,
			Snapshot: true,
			Token:    token,
		},
	})
}

func addParams(value any) (trading.AddParams, bool) {
	switch typed := value.(type) {
	case trading.AddParams:
		return typed, true
	case *trading.AddParams:
		if typed == nil {
			return trading.AddParams{}, false
		}

		return *typed, true
	default:
		return trading.AddParams{}, false
	}
}
