package user

const (
	BalanceSnapshot = "snapshot"
	BalanceUpdate   = "update"
)

/*
SubscribeFrame is the Kraken WebSocket v2 subscribe request for balances.
See https://docs.kraken.com/api/docs/websocket-v2/balances
*/
type SubscribeFrame struct {
	Method string        `json:"method"`
	Params BalanceParams `json:"params"`
}

/*
BalanceParams is the Kraken WebSocket v2 subscribe payload for the balances channel.
*/
type BalanceParams struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
	Rebased  bool   `json:"rebased,omitempty"`
	Users    string `json:"users,omitempty"`
	Token    string `json:"token,omitempty"`
}

/*
BalanceWallet is one wallet holding for an asset in a balances snapshot.
*/
type BalanceWallet struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Balance float64 `json:"balance"`
}

/*
Balance is one asset row from the balances channel snapshot or update.
*/
type Balance struct {
	Asset      string          `json:"asset"`
	AssetClass string          `json:"asset_class"`
	Balance    float64         `json:"balance"`
	Wallets    []BalanceWallet `json:"wallets,omitempty"`
	Amount     float64         `json:"amount,omitempty"`
	Fee        float64         `json:"fee,omitempty"`
	LedgerID   string          `json:"ledger_id,omitempty"`
	RefID      string          `json:"ref_id,omitempty"`
	Timestamp  string          `json:"timestamp,omitempty"`
	Type       string          `json:"type,omitempty"`
	Subtype    string          `json:"subtype,omitempty"`
	Category   string          `json:"category,omitempty"`
	WalletType string          `json:"wallet_type,omitempty"`
	WalletID   string          `json:"wallet_id,omitempty"`
	User       string          `json:"user,omitempty"`
	Envelope   string          `json:"-"`
}

type Balances struct {
	Asset       []Balance          `json:"asset"`
	Currency    string             `json:"Currency,omitempty"`
	Balance     float64            `json:"Balance,omitempty"`
	Inventory   map[string]float64 `json:"Inventory,omitempty"`
	AvgEntry    map[string]float64 `json:"AvgEntry,omitempty"`
	Marks       map[string]float64 `json:"Marks,omitempty"`
	Expected    map[string]float64 `json:"ExpectedExit,omitempty"`
	Unrealized  map[string]float64 `json:"Unrealized,omitempty"`
	ExitFeeRate map[string]float64 `json:"ExitFeeRate,omitempty"`
	Realized    float64            `json:"Realized,omitempty"`
}
