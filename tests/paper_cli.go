package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

/*
InstallPaperCLI drops a stateful `kraken` shim ahead of PATH so Session paper
Buy/Sell/Balance round-trips without touching the operator's real paper wallet.
Prices are fixed unless SetPaperPrice is used via the state file.
*/
func InstallPaperCLI(testingTB testing.TB, statePath string) {
	testingTB.Helper()

	dir := testingTB.TempDir()
	bin := filepath.Join(dir, "kraken")
	script := `#!/usr/bin/env python3
import json, os, sys, time
from pathlib import Path

STATE = Path(os.environ["SYMM_PAPER_STATE"])

def load():
    if STATE.exists():
        return json.loads(STATE.read_text())
    return {"USD": 10000.0, "assets": {}, "prices": {"MATIC/USD": 1.0, "MATICUSD": 1.0}, "n": 0}

def save(state):
    STATE.write_text(json.dumps(state))

def normalize(pair):
    return pair if "/" in pair else (pair[:-3] + "/" + pair[-3:] if len(pair) > 3 else pair)

def price(state, pair):
    key = normalize(pair)
    return float(state["prices"].get(key) or state["prices"].get(pair) or 1.0)

def balance_payload(state):
    balances = {
        "USD": {
            "available": float(state["USD"]),
            "reserved": 0.0,
            "total": float(state["USD"]),
        }
    }
    for asset, qty in state.get("assets", {}).items():
        qty = float(qty)
        if qty <= 0:
            continue
        balances[asset] = {"available": qty, "reserved": 0.0, "total": qty}
    print(json.dumps({"balances": balances, "mode": "paper"}))

def fill(state, side, pair, volume):
    pair = normalize(pair)
    volume = float(volume)
    px = price(state, pair)
    base = pair.split("/")[0]
    cost = volume * px
    fee = cost * 0.0026
    state["n"] = int(state.get("n", 0)) + 1
    n = state["n"]
    assets = state.setdefault("assets", {})
    if side == "buy":
        if state["USD"] < cost + fee:
            print(json.dumps({"error": "insufficient", "message": "Insufficient USD"}), file=sys.stderr)
            sys.exit(1)
        state["USD"] -= cost + fee
        assets[base] = float(assets.get(base, 0.0)) + volume
    else:
        have = float(assets.get(base, 0.0))
        if have + 1e-12 < volume:
            print(json.dumps({"error": "insufficient", "message": "Insufficient " + base}), file=sys.stderr)
            sys.exit(1)
        assets[base] = have - volume
        state["USD"] += cost - fee
    save(state)
    print(json.dumps({
        "action": "market_order_filled",
        "order_id": f"PAPER-{n:05d}a",
        "trade_id": f"PAPER-{n:05d}b",
        "pair": pair,
        "side": side,
        "volume": volume,
        "price": px,
        "cost": cost,
        "fee": fee,
        "status": "filled",
        "time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }))

def main():
    if len(sys.argv) < 2 or sys.argv[1] != "paper":
        sys.exit(2)
    state = load()
    cmd = sys.argv[2] if len(sys.argv) > 2 else ""
    if cmd == "balance":
        balance_payload(state)
        return
    if cmd in ("buy", "sell"):
        fill(state, cmd, sys.argv[3], sys.argv[4])
        return
    if cmd == "history":
        print(json.dumps({"trades": []}))
        return
    sys.exit(2)

if __name__ == "__main__":
    main()
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		testingTB.Fatal(err)
	}

	if err := os.WriteFile(statePath, []byte(
		`{"USD":50000.0,"assets":{},"prices":{"MATIC/USD":1.0},"n":0}`,
	), 0o644); err != nil {
		testingTB.Fatal(err)
	}

	testingTB.Setenv("SYMM_PAPER_STATE", statePath)
	testingTB.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

/*
SetPaperPrice updates the shim's mark for symbol so a later sell can lock a
profit relative to entry.
*/
func SetPaperPrice(testingTB testing.TB, statePath, symbol string, mark float64) {
	testingTB.Helper()
	mutatePaperState(testingTB, statePath, func(state map[string]any) {
		prices, _ := state["prices"].(map[string]any)

		if prices == nil {
			prices = map[string]any{}
			state["prices"] = prices
		}

		prices[symbol] = mark
	})
}

/*
SetPaperCash aligns the shim wallet with SeedQuoteCapital so Desk sizing and
paper AddOrder spend the same quote pool.
*/
func SetPaperCash(testingTB testing.TB, statePath string, cash float64) {
	testingTB.Helper()
	mutatePaperState(testingTB, statePath, func(state map[string]any) {
		state["USD"] = cash
	})
}

/*
SetPaperAsset sets a base-asset inventory row on the shim wallet.
*/
func SetPaperAsset(testingTB testing.TB, statePath, asset string, qty float64) {
	testingTB.Helper()
	mutatePaperState(testingTB, statePath, func(state map[string]any) {
		assets, _ := state["assets"].(map[string]any)

		if assets == nil {
			assets = map[string]any{}
			state["assets"] = assets
		}

		assets[asset] = qty
	})
}

func mutatePaperState(
	testingTB testing.TB,
	statePath string,
	mutate func(map[string]any),
) {
	testingTB.Helper()

	raw, err := os.ReadFile(statePath)

	if err != nil {
		testingTB.Fatal(err)
	}

	var state map[string]any

	if err := json.Unmarshal(raw, &state); err != nil {
		testingTB.Fatal(err)
	}

	mutate(state)

	out, err := json.Marshal(state)

	if err != nil {
		testingTB.Fatal(err)
	}

	if err := os.WriteFile(statePath, out, 0o644); err != nil {
		testingTB.Fatal(err)
	}
}
