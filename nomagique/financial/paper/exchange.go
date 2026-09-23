package paper

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
ExchangeServer wires one venue to one account. A write that carries a
decision is the program committing feedback after the evaluation: it re-carries
that evaluation's frame, which the venue has already applied, so only the
decision is taken from it.
*/
type ExchangeServer struct {
	venue   *venue
	account *account
}

func NewExchange() *ExchangeServer {
	return &ExchangeServer{venue: newVenue(), account: newAccount()}
}

func (server *ExchangeServer) Write(ctx context.Context, call Exchange_write) error {
	args := call.Args()
	capital, err := args.Capital()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: capital", err))
	}
	schedule, err := args.Schedule()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: schedule", err))
	}

	if err := server.account.open(capital, schedule); err != nil {
		return err
	}
	moment, err := args.Time()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: time", err))
	}
	action, err := args.Action()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: action", err))
	}

	if action != "" {
		return server.decide(args, action, moment)
	}
	frame, err := args.Frame()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: frame", err))
	}
	executed, err := server.venue.apply(frame, args.Depth(), moment)

	if err != nil {
		return err
	}

	for _, fill := range executed {
		if err := server.account.settle(fill); err != nil {
			return err
		}
	}
	return nil
}

func (server *ExchangeServer) decide(args Exchange_write_Params, action, moment string) error {
	symbol, err := args.Symbol()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: symbol", err))
	}
	currency, err := args.Currency()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: currency", err))
	}

	if currency == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: currency is required", nil))
	}
	text, err := args.Fraction()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: fraction", err))
	}
	fraction, err := amount(text, "fraction")

	if err != nil {
		return err
	}

	if fraction.Cmp(one()) > 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "exchange: fraction cannot exceed the cash", nil))
	}
	placed, err := server.account.decide(symbol, action, moment, terms{currency: currency, fraction: fraction})

	if err != nil || placed == nil {
		return err
	}
	return server.venue.rest(*placed)
}

/* Done reports the oldest unreported outcome and the account as it stands. */
func (server *ExchangeServer) Done(ctx context.Context, call Exchange_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "exchange: allocate result", err))
	}
	wallet := server.account
	cash, available := zero(), zero()

	if wallet.cash != nil {
		cash, available = wallet.cash, wallet.available()
	}
	result.SetResting(uint64(len(server.venue.resting)))
	result.SetBooks(uint64(len(server.venue.books)))
	symbols := wallet.held()
	holding, err := result.NewHolding(int32(len(symbols)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "exchange: allocate holdings", err))
	}

	for index, symbol := range symbols {
		if err := holding.Set(index, symbol); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "exchange: emit holding", err))
		}
	}

	if err := first(
		result.SetCash(cash.String()),
		result.SetAvailable(available.String()),
		result.SetVolume(wallet.traded().String()),
		emitOutcome(result, wallet),
	); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "exchange: emit account", err))
	}
	return nil
}

func emitOutcome(result Account, wallet *account) error {
	reported, found := wallet.report()

	if !found {
		result.SetQuiet()
		return nil
	}

	if reported.refused != nil {
		result.SetRefused()
		refused := result.Refused()
		return first(
			refused.SetRefusedSymbol(reported.refused.symbol),
			refused.SetAction(reported.refused.action),
			refused.SetReason(reported.refused.reason),
		)
	}
	result.SetClosed()
	closed := result.Closed()
	trip := reported.closed
	closed.SetReturn(trip.ratio)
	return first(
		closed.SetSymbol(trip.symbol),
		closed.SetOpened(trip.opened),
		closed.SetClosedAt(trip.closed),
		closed.SetBasis(trip.basis.String()),
		closed.SetProceeds(trip.proceeds.String()),
		closed.SetPnl(trip.pnl.String()),
	)
}

func first(results ...error) error {
	for _, err := range results {
		if err != nil {
			return err
		}
	}
	return nil
}
