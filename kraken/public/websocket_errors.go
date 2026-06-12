package public

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
)

func (ws *WebSocket) handleErrors(message *types.SocketMessage) {
	for _, errorText := range message.Errors {
		exchangeError := wsutil.ParseExchangeError(errorText)
		decision := wsutil.DefaultExchangeErrorPolicy().Classify(exchangeError)
		observability.Shared().RecordExchangeError(
			"kraken/public",
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
