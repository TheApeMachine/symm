package broker

/*
Ledger names the concurrent trading book a lot belongs to.
*/
type Ledger string

const (
	LedgerVirtual Ledger = "virtual"
	LedgerPaper   Ledger = "paper"
	LedgerReal    Ledger = "real"
)

func (ledger Ledger) String() string {
	return string(ledger)
}

func (ledger Ledger) Virtual() bool {
	return ledger == LedgerVirtual
}

func ParseLedger(raw string, virtual bool) Ledger {
	switch raw {
	case string(LedgerVirtual), string(LedgerPaper), string(LedgerReal):
		return Ledger(raw)
	}

	if virtual {
		return LedgerVirtual
	}

	return LedgerPaper
}

/*
HistoryKey is the S3 prefix stem for open/closed lots on this ledger.
*/
func (ledger Ledger) HistoryKey() string {
	switch ledger {
	case LedgerVirtual:
		return "virtual"
	case LedgerReal:
		return "real"
	default:
		return "paper"
	}
}

/*
PendingKey scopes settle handoff so concurrent ledgers do not clobber.
*/
func (ledger Ledger) PendingKey(symbol string) string {
	return ledger.HistoryKey() + ":" + symbol
}
