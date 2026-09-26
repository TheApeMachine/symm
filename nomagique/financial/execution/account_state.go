package execution

import (
	"context"
	"sort"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* emit projects exact retained facts into the public node schema. */
func (server *AccountServer) emit(result AccountState) error {
	result.SetLive(server.live)
	for _, err := range []error{result.SetPhase(server.phase()), result.SetReason(server.reason)} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	if server.cash == nil {
		return nil
	}
	result.SetObservations(server.observations)
	result.SetRevision(server.revision)
	result.SetLive(server.live)
	result.SetDurableRevision(server.durableRevision)
	if err := result.SetPersistenceError(server.persistenceError); err != nil {
		return errnie.Error(err)
	}
	result.SetEpoch(server.epoch)
	result.SetSequence(server.sequence)
	result.SetDecisions(server.decisions)
	result.SetOutcomes(server.outcomes)
	result.SetPositives(server.positives)
	result.SetMeanEdge(server.mean)
	result.SetEdgeM2(server.m2)
	result.SetEdgeDefined(server.outcomes > 0)
	result.SetUncertaintyDefined(server.outcomes > 1)
	result.SetStandardError(server.uncertainty())
	for _, err := range []error{result.SetInitialCash(server.configuration), result.SetCash(server.cash.String()), result.SetPnl(server.pnl.String()), result.SetPhase(server.phase()), result.SetSymbol(server.symbol), result.SetDecision(server.decision), result.SetReason(server.reason)} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	names := make([]string, 0, len(server.positions))
	equity, defined := server.cash.Copy(), true
	var open uint64
	for name, position := range server.positions {
		if position.quantity.Sign() == 0 && position.pending == "" {
			continue
		}
		names = append(names, name)
		equity = equity.SetScale(max(equity.GetScale(), position.reserved.GetScale())).Add(position.reserved)

		if position.quantity.Sign() == 0 {
			continue
		}
		open++

		if position.mark == nil {
			defined = false
			continue
		}
		equity = equity.SetScale(max(equity.GetScale(), position.mark.GetScale())).Add(position.mark)
	}
	result.SetOpen(open)

	if defined {
		if err := result.SetEquity(equity.String()); err != nil {
			return errnie.Error(err)
		}
	}
	sort.Strings(names)
	positions, err := result.NewPositions(int32(len(names)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, name := range names {
		if err := server.positions[name].emit(positions.At(index), name); err != nil {
			return err
		}
	}

	if server.closed == nil {
		return nil
	}
	closed, err := result.NewClosed()

	if err != nil {
		return errnie.Error(err)
	}
	closed.SetEdge(server.closed.edge)
	closed.SetEpoch(server.closed.epoch)
	closed.SetSequence(server.closed.sequence)
	for _, err := range []error{closed.SetSymbol(server.closed.symbol), closed.SetBasis(server.closed.basis.String()), closed.SetProceeds(server.closed.proceeds.String()), closed.SetPnl(server.closed.pnl.String()), closed.SetOpened(server.closed.opened), closed.SetClosed(server.closed.closed)} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	record, err := result.NewRecord()

	if err != nil {
		return errnie.Error(err)
	}
	record.SetTypeId(RoundTrip_TypeID)
	return errnie.Error(record.SetValue(capnp.Struct(closed).ToPtr()))
}

/* emit retains enough exact position facts to resume one pending order. */
func (position *positionState) emit(result Position, name string) error {
	result.SetEpoch(position.epoch)
	result.SetSequence(position.sequence)
	result.SetCostPlaces(position.costPlaces)
	result.SetIntentRevision(position.intentRevision)
	for _, err := range []error{result.SetSymbol(name), result.SetPending(position.pending), result.SetOpened(position.opened), result.SetOrderId(position.orderId), result.SetClientId(position.clientId)} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	for _, field := range []struct {
		value *decimal.Decimal
		set   func(string) error
	}{
		{position.quantity, result.SetQuantity}, {position.basis, result.SetBasis}, {position.spent, result.SetSpent},
		{position.proceeds, result.SetProceeds}, {position.mark, result.SetMark}, {position.amount, result.SetAmount},
		{position.reserved, result.SetReserved}, {position.fee, result.SetFee}, {position.minimumQuantity, result.SetMinimumQuantity},
		{position.minimumCost, result.SetMinimumCost}, {position.increment, result.SetIncrement},
		{position.filledQuantity, result.SetFilledQuantity}, {position.filledCost, result.SetFilledCost}, {position.filledFee, result.SetFilledFee},
	} {
		if field.value == nil {
			continue
		}

		if err := field.set(field.value.String()); err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* Snapshot transfers the ledger through the existing native checkpoint protocol. */
func (server *AccountServer) Snapshot(ctx context.Context, call runtime.Snapshot_snapshot) error {
	encoded, err := server.snapshotBytes()
	if err != nil {
		return err
	}
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetData(encoded))
}

/* snapshotBytes encodes one immutable native account revision. */
func (server *AccountServer) snapshotBytes() ([]byte, error) {
	if server.cash == nil {
		return nil, nil
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, errnie.Error(err)
	}
	defer message.Release()
	state, err := NewRootAccountState(segment)
	if err != nil {
		return nil, errnie.Error(err)
	}
	if err := server.emit(state); err != nil {
		return nil, err
	}
	encoded, err := message.Marshal()
	return encoded, errnie.Error(err)
}

/* Restore validates complete retained facts before replacing the active ledger. */
func (server *AccountServer) Restore(ctx context.Context, call runtime.Snapshot_restore) error {
	encoded, err := call.Args().Data()

	if err != nil {
		return errnie.Error(err)
	}
	return server.restoreBytes(encoded)
}

/* restoreBytes validates a native revision and preserves configured capabilities. */
func (server *AccountServer) restoreBytes(encoded []byte) error {
	if len(encoded) == 0 {
		*server = *NewAccount()
		return nil
	}
	message, err := capnp.Unmarshal(encoded)

	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	state, err := ReadRootAccountState(message)

	if err != nil {
		return errnie.Error(err)
	}
	restored := NewAccount()
	restored.configuration, err = state.InitialCash()

	if err != nil {
		return errnie.Error(err)
	}

	if restored.configuration == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: snapshot has no initial capital", nil))
	}
	for _, field := range []struct {
		read   func() (string, error)
		target **decimal.Decimal
		name   string
	}{
		{state.InitialCash, &restored.initial, "initial cash"}, {state.Cash, &restored.cash, "cash"}, {state.Pnl, &restored.pnl, "pnl"},
	} {
		*field.target, err = amountArgument(field.read, field.name)

		if err != nil {
			return err
		}
	}

	if restored.initial.Sign() <= 0 || restored.cash.Sign() < 0 || state.Positives() > state.Outcomes() {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: invalid cash or outcome counters", nil))
	}
	restored.observations, restored.decisions = state.Observations(), state.Decisions()
	restored.live, restored.revision = state.Live(), state.Revision()
	restored.durableRevision = state.Revision()
	restored.epoch, restored.sequence = state.Epoch(), state.Sequence()
	if restored.observations > 0 && (restored.epoch <= 0 || restored.sequence < 0) {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: snapshot omitted its last causal stamp", nil))
	}

	restored.outcomes, restored.positives, restored.mean, restored.m2 = state.Outcomes(), state.Positives(), state.MeanEdge(), state.EdgeM2()
	positions, err := state.Positions()

	if err != nil {
		return errnie.Error(err)
	}
	for index := range positions.Len() {
		item := positions.At(index)
		name, err := item.Symbol()

		if err != nil {
			return errnie.Error(err)
		}

		if name == "" || restored.positions[name] != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "execution account: empty or duplicate snapshot market", nil))
		}
		position := &positionState{epoch: item.Epoch(), sequence: item.Sequence(), costPlaces: item.CostPlaces(), intentRevision: item.IntentRevision()}

		if err := position.restore(item); err != nil {
			return err
		}
		restored.positions[name] = position
	}
	if server.cash != nil && server.revision > restored.revision {
		return nil
	}
	restored.checkpoint, restored.checkpointKey = server.checkpoint, server.checkpointKey
	restored.authorized = server.authorized
	restored.symbol, restored.moment = server.symbol, server.moment
	*server = *restored
	return nil
}

/* restore validates the monetary and pending-order invariants of one position. */
func (position *positionState) restore(item Position) error {
	var err error
	for _, field := range []struct {
		read   func() (string, error)
		target **decimal.Decimal
		name   string
	}{
		{item.Quantity, &position.quantity, "quantity"}, {item.Basis, &position.basis, "basis"}, {item.Spent, &position.spent, "spent"},
		{item.Proceeds, &position.proceeds, "proceeds"}, {item.Amount, &position.amount, "amount"}, {item.Reserved, &position.reserved, "reserved"},
		{item.Fee, &position.fee, "fee"}, {item.MinimumQuantity, &position.minimumQuantity, "minimum quantity"},
		{item.MinimumCost, &position.minimumCost, "minimum cost"}, {item.Increment, &position.increment, "increment"},
	} {
		*field.target, err = amountArgument(field.read, field.name)

		if err != nil {
			return err
		}

		if (*field.target).Sign() < 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "execution account: negative retained "+field.name, nil))
		}
	}
	position.pending, err = item.Pending()

	if err != nil {
		return errnie.Error(err)
	}
	position.opened, err = item.Opened()

	if err != nil {
		return errnie.Error(err)
	}

	if position.pending != "" && position.pending != "buy" && position.pending != "sell" || position.epoch <= 0 || position.sequence < 0 || position.increment.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: invalid retained order or stamp", nil))
	}
	position.orderId, err = item.OrderId()
	if err != nil {
		return errnie.Error(err)
	}
	position.clientId, err = item.ClientId()
	if err != nil {
		return errnie.Error(err)
	}
	if position.clientId != "" {
		position.attempted = true
		for _, field := range []struct {
			read   func() (string, error)
			target **decimal.Decimal
			name   string
		}{{item.FilledQuantity, &position.filledQuantity, "filled quantity"}, {item.FilledCost, &position.filledCost, "filled cost"}, {item.FilledFee, &position.filledFee, "filled fee"}} {
			*field.target, err = amountArgument(field.read, field.name)
			if err != nil {
				return err
			}
		}
	}
	// Restored inventory waits for a newly reconciled live book before it has a mark.
	return nil
}

/* phase reflects the authored mode and permission rather than inferring edge. */
func (server *AccountServer) phase() string {
	if !server.live {
		return "PAPER_LEARNING"
	}
	if !server.authorized {
		return "LIVE_PROTECTION"
	}
	if server.persistenceError != "" {
		return "LIVE_DURABILITY_BLOCKED"
	}
	return "LIVE_AUTHORIZED"
}
