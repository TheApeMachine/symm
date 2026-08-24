import pathlib

p = pathlib.Path('broker/desk.go')
t = p.read_text()

t = t.replace('\tui             *transport.MapReduce[*types.UIFrame]',
              '\tui             *runtime.Channel[*types.UIFrame]')
t = t.replace('\twork           *transport.Consumer[*types.Symbol]',
              '\ttickerWork     *runtime.Subscription[kraken.TickerData]\n\texecutionWork  *runtime.Subscription[kraken.ExecutionData]')

t = t.replace('''	recorder *audit.Recorder,
	store *PositionStore,
	ui *transport.MapReduce[*types.UIFrame],
) *Desk {''', '''	recorder *audit.Recorder,
	store *PositionStore,
	bus *runtime.Workspace,
) *Desk {''')

old_reg = '''	desk.recovery = NewRecovery(
		ctx, api, ui, instrument, price, balance, recorder, store, desk.positions,
	)
	desk.work = transport.NewStepConsumer(
		desk.Name(),
		func(symbol *types.Symbol) string {
			if symbol == nil {
				return ""
			}
			return symbol.Symbol
		},
		desk.Step,
	)
	thesis.Work(types.SourceDesk).Register(desk.work)
'''
new_reg = '''	desk.ui = runtime.ChannelOf[*types.UIFrame](
		bus, types.ChannelUI,
		func(frame *types.UIFrame) string { return "" },
	)
	desk.recovery = NewRecovery(
		ctx, api, desk.ui, instrument, price, balance, recorder, store, desk.positions,
	)
	desk.tickerWork = runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	).Subscribe(desk.Name(), desk.StepTicker)
	desk.executionWork = runtime.ChannelOf[kraken.ExecutionData](
		bus, types.ChannelExecutions,
		func(execution kraken.ExecutionData) string { return execution.Symbol },
	).Subscribe(desk.Name(), desk.StepExecution)
'''
if old_reg not in t:
    print("DESK REG NOT FOUND")
else:
    t = t.replace(old_reg, new_reg, 1)

old_step = '''// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (desk *Desk) Step(symbol *types.Symbol) error {
	if symbol == nil {
		return nil
	}
	err := desk.consumeSymbol(symbol)
	desk.err = err
	if err != nil && desk.thesis != nil {
		desk.thesis.Fail(err)
	}
	return err
}

func (desk *Desk) consumeSymbol(symbol *types.Symbol) error {
	drained := false

	for ticker := range symbol.MarketTickers(
		symbol.TickerConsumers[types.TickerConsumerDesk],
	) {
		drained = true
		desk.price.Update(&ticker)
		found, ok := desk.positions.Load(symbol.Symbol)

		if ok && found != nil {
			position, ok := found.(*Position)

			if ok && position != nil {
				position.onTicker(ticker)

				if observer, observesMarks := desk.equityObserver.(MarkObserver); observesMarks {
					err := observer.ObserveMark(position.MarkFeedback(ticker.Timestamp))

					if err != nil {
						return errnie.Error(err)
					}
				}
			}
		}
	}

	for execution := range symbol.MarketExecutions(
		symbol.ExecutionConsumers[types.ExecutionConsumerDesk],
	) {
		drained = true
		found, ok := desk.positions.Load(symbol.Symbol)

		if !ok || found == nil {
			continue
		}

		position, ok := found.(*Position)

		if !ok || position == nil {
			continue
		}

		if position.onExecution(kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data:    []kraken.ExecutionData{execution},
		}) {
			desk.foldPassage(position)
			desk.positions.CompareAndDelete(symbol.Symbol, position)
		}
	}

	return nil
}'''
new_step = '''// StepTicker advances one ticker observation: the price cache and any open
// position's live mark.
func (desk *Desk) StepTicker(ticker kraken.TickerData) error {
	desk.price.Update(&ticker)
	found, ok := desk.positions.Load(ticker.Symbol)

	if !ok || found == nil {
		return nil
	}

	position, ok := found.(*Position)

	if !ok || position == nil {
		return nil
	}

	position.onTicker(ticker)

	if observer, observesMarks := desk.equityObserver.(MarkObserver); observesMarks {
		return errnie.Error(observer.ObserveMark(position.MarkFeedback(ticker.Timestamp)))
	}

	return nil
}

// StepExecution advances one execution against the symbol's open position.
func (desk *Desk) StepExecution(execution kraken.ExecutionData) error {
	found, ok := desk.positions.Load(execution.Symbol)

	if !ok || found == nil {
		return nil
	}

	position, ok := found.(*Position)

	if !ok || position == nil {
		return nil
	}

	if position.onExecution(kraken.Execution{
		Channel: "executions",
		Type:    "update",
		Data:    []kraken.ExecutionData{execution},
	}) {
		desk.foldPassage(position)
		desk.positions.CompareAndDelete(execution.Symbol, position)
	}

	return nil
}'''
if old_step not in t:
    print("DESK STEP SECTION NOT FOUND")
else:
    t = t.replace(old_step, new_step, 1)

t = t.replace('''	desk.thesis.Publish(&wire.FrameT{
		Type: wire.FrameEquityFrame,
		Value: &wire.EquityFrameT{
			Cash:       desk.balance.Cash().String(),
			Unrealized: tradeBalance.UnrealizedPnL.String(),
			Equity:     tradeBalance.Equity.String(),
		},
	})''', '''	if desk.ui != nil {
		desk.ui.Publish(&types.UIFrame{
			Type: wire.FrameEquityFrame,
			Value: &wire.EquityFrameT{
				Cash:       desk.balance.Cash().String(),
				Unrealized: tradeBalance.UnrealizedPnL.String(),
				Equity:     tradeBalance.Equity.String(),
			},
		})
	}''')

t = t.replace('''		desk.thesis.Symbol(decision.Symbol).Positions.Push(position)
''', '')

start = t.find('// consume is retained as a compatibility entry point for deterministic tests.')
if start >= 0:
    end = t.find('\n}\n', t.find('group.Wait()', start))
    if end >= 0:
        t = t[:start] + t[end + 3:]
        print("desk consume removed")
    else:
        print("desk consume end not found")
else:
    print("desk consume not present")

p.write_text(t)
print("desk transformed")
