package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
	FundingLedger observes external quote flows from Kraken's paginated ledger.

The cursor overlaps the final API second; ledger IDs prevent double counting.
Non-quote transfers lack a historical quote valuation and explicitly make
funding unavailable. A new connection session starts a new funding reference.
Protocol: https://docs.kraken.com/api-reference/account-data/get-ledgers-info
*/
type FundingLedger struct {
	mu     sync.Mutex
	cursor int64
	seen   map[string]struct{}
	total  *decimal.Decimal
	reason string
}

/* ledgerRequest uses the venue's inclusive Unix-second range and result offset. */
type ledgerRequest struct {
	Start  int64 `json:"start"`
	End    int64 `json:"end"`
	Offset int   `json:"ofs"`
}

/* MarshalJSON supplies the existing authenticated REST request boundary. */
func (request ledgerRequest) MarshalJSON() ([]byte, error) {
	type plain ledgerRequest
	return json.Marshal(plain(request))
}

/* ledgerEntry contains the actual cash movement and its venue timestamp. */
type ledgerEntry struct {
	At     float64          `json:"time"`
	Kind   string           `json:"type"`
	Asset  string           `json:"asset"`
	Amount *decimal.Decimal `json:"amount"`
	Fee    *decimal.Decimal `json:"fee"`
}

/* Observe fetches a complete bounded range before advancing the funding cursor. */
func (funding *FundingLedger) Observe(post func(string, json.Marshaler) ([]byte, error), normalize func(string) string, quote string, at time.Time) (*decimal.Decimal, string, error) {
	funding.mu.Lock()
	defer funding.mu.Unlock()

	if funding.total == nil {
		funding.total = decimal.NewFromInt64(0)
		funding.cursor = at.Unix()
		funding.seen = make(map[string]struct{})
	}
	request := ledgerRequest{Start: funding.cursor, End: at.Unix()}
	entries := make(map[string]ledgerEntry)
	for {
		payload, err := post("/0/private/Ledgers", request)

		if err != nil {
			return nil, "funding ledger unavailable", err
		}
		response := struct {
			Error  []string `json:"error"`
			Result struct {
				Ledger map[string]ledgerEntry `json:"ledger"`
				Count  *int                   `json:"count"`
			} `json:"result"`
		}{}

		if err := json.Unmarshal(payload, &response); err != nil {
			return nil, "invalid funding ledger", err
		}

		if len(response.Error) > 0 {
			return nil, "funding ledger unavailable", fmt.Errorf("funding ledger: %s", strings.Join(response.Error, "; "))
		}

		if response.Result.Ledger == nil || response.Result.Count == nil || *response.Result.Count < 0 {
			return nil, "incomplete funding ledger", fmt.Errorf("funding ledger: complete entries and count required")
		}

		for identity, entry := range response.Result.Ledger {
			entries[identity] = entry
		}
		request.Offset += len(response.Result.Ledger)

		if request.Offset >= *response.Result.Count {
			break
		}

		if len(response.Result.Ledger) == 0 {
			return nil, "incomplete funding ledger", fmt.Errorf("funding ledger: empty page before advertised count")
		}
	}
	for identity, entry := range entries {
		if entry.Kind != "trade" && entry.Kind != "margin" && entry.Kind != "rollover" && (entry.Amount == nil || entry.Fee == nil) {
			return nil, "invalid funding ledger", fmt.Errorf("funding ledger %s: missing amount or fee", identity)
		}
	}
	next := make(map[string]struct{})
	for identity, entry := range entries {
		if int64(entry.At) >= request.End {
			next[identity] = struct{}{}
		}

		if _, observed := funding.seen[identity]; observed {
			continue
		}

		if entry.Kind == "trade" || entry.Kind == "margin" || entry.Kind == "rollover" {
			continue
		}

		if normalize(entry.Asset) != quote {
			funding.reason = "non-quote funding requires historical valuation: " + entry.Asset
			continue
		}
		funding.total = funding.total.Add(entry.Amount).Sub(entry.Fee)
	}
	funding.cursor, funding.seen = request.End, next

	if funding.reason != "" {
		return nil, funding.reason, nil
	}
	return funding.total.Copy(), "", nil
}
