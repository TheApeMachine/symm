#!/usr/bin/env python3
"""
attribution.py — realized-edge attribution and performance metrics for symm.

Consumes the audit JSONL sidecars in runs/ (or paths passed as args) and
reconstructs every round-trip trade, attributing realized PnL to the signal
sources that authorized each entry.

    python3 analysis/attribution.py                 # scan ./runs/audit-*.jsonl
    python3 analysis/attribution.py runs/audit-X.jsonl ...
    python3 analysis/attribution.py --report analysis/PERFORMANCE.md

Per-signal attribution requires the `contributions` field on trade_entry_fill
(added 2026-05-29). Runs logged before that only support portfolio- and
perspective-level metrics; the script degrades gracefully and says so.

Fee assumption: DefaultTakerFeePct = 0.26% per side (config/config.go). Override
with --taker-fee-pct.
"""
import argparse
import glob
import json
import math
import os
import sys
from collections import defaultdict

PERSPECTIVE_NAMES = {
    0: "microstructure",
    1: "flow",
    2: "cross_asset",
    3: "sentiment",
}


def load_events(paths):
    wanted = (
        '"trade_entry_fill"',
        '"trade_exit_fill"',
        '"measurement_ingest"',
        '"trade_entry_skip"',
    )
    entries, exits, skips = [], [], []
    msg_sources = defaultdict(int)
    for path in paths:
        with open(path) as fh:
            for line in fh:
                if not any(w in line for w in wanted):
                    continue
                try:
                    rec = json.loads(line)
                except json.JSONDecodeError:
                    continue
                ev = rec.get("event")
                if ev == "trade_entry_fill":
                    entries.append(rec)
                elif ev == "trade_exit_fill":
                    exits.append(rec)
                elif ev == "trade_entry_skip":
                    skips.append(rec)
                elif ev == "measurement_ingest":
                    msg_sources[rec.get("source", "")] += 1
    return entries, exits, skips, msg_sources


def pair_trades(entries, exits, taker_fee):
    """FIFO-pair each entry to the first later exit of the same symbol."""
    exits_by = defaultdict(list)
    for x in sorted(exits, key=lambda r: r["ts"]):
        exits_by[x["symbol"]].append(x)
    cursor = defaultdict(int)
    trades = []
    for e in sorted(entries, key=lambda r: r["ts"]):
        sym = e["symbol"]
        lst = exits_by[sym]
        match = None
        for i in range(cursor[sym], len(lst)):
            if lst[i]["ts"] > e["ts"]:
                match = lst[i]
                cursor[sym] = i + 1
                break
        if match is None:
            continue
        qty, ep, xp = e["qty"], e["price"], match["price"]
        gross = (xp - ep) * qty
        fees = (ep + xp) * qty * taker_fee
        trades.append(
            {
                "symbol": sym,
                "entry_ts": e["ts"],
                "exit_ts": match["ts"],
                "entry_price": ep,
                "exit_price": xp,
                "qty": qty,
                "notional": ep * qty,
                "gross": gross,
                "fees": fees,
                "net": gross - fees,
                "realized_ret": (xp - ep) / ep if ep else 0.0,
                "predicted_return": e.get("predicted_return"),
                "confidence": e.get("confidence"),
                "exit_reason": match.get("reason", ""),
                "perspective_type": e.get("perspective_type"),
                "dominant_source": e.get("dominant_source"),
                "contributions": e.get("contributions") or {},
            }
        )
    return trades


def summarize(trades):
    if not trades:
        return "No round-trip trades found.\n"
    n = len(trades)
    net = sum(t["net"] for t in trades)
    gross = sum(t["gross"] for t in trades)
    fees = sum(t["fees"] for t in trades)
    notional = sum(t["notional"] for t in trades)
    net_wins = sum(1 for t in trades if t["net"] > 0)
    gross_wins = sum(1 for t in trades if t["gross"] > 0)
    avg_pred = sum((t["predicted_return"] or 0) for t in trades) / n
    avg_real = sum(t["realized_ret"] for t in trades) / n
    out = []
    out.append("## Portfolio (all round-trips)\n")
    out.append(f"- Round-trips paired: **{n}**")
    out.append(
        f"- Net PnL after fees: **{net:+.4f} EUR** "
        f"({100 * net / notional:+.3f}% of notional traded)"
    )
    out.append(f"- Gross PnL pre-fee: {gross:+.4f} EUR")
    out.append(f"- Fees paid: {fees:.4f} EUR")
    out.append(f"- Net win rate: **{net_wins}/{n} = {100 * net_wins / n:.0f}%**")
    out.append(
        f"- Gross win rate (pre-fee): {gross_wins}/{n} = {100 * gross_wins / n:.0f}%"
    )
    out.append(f"- Avg predicted return: {100 * avg_pred:.3f}%")
    out.append(f"- Avg realized move: {100 * avg_real:.3f}%")
    if avg_real:
        out.append(
            f"- Prediction optimism ratio: {avg_pred / avg_real:.1f}x"
            if avg_real
            else ""
        )
    out.append("")
    return "\n".join(out) + "\n"


def group_block(title, key_fn, trades):
    buckets = defaultdict(lambda: {"n": 0, "net": 0.0, "wins": 0})
    for t in trades:
        for key in key_fn(t):
            b = buckets[key]
            b["n"] += 1
            b["net"] += t["net"]
            if t["net"] > 0:
                b["wins"] += 1
    if not buckets:
        return ""
    rows = sorted(buckets.items(), key=lambda kv: kv[1]["net"])
    out = [f"## {title}\n", "| key | trades | win% | net EUR |", "|---|---:|---:|---:|"]
    for key, b in rows:
        out.append(
            f"| {key} | {b['n']} | {100 * b['wins'] / b['n']:.0f}% | {b['net']:+.4f} |"
        )
    return "\n".join(out) + "\n\n"


def attribution_block(trades):
    """Per-signal realized-edge attribution via the contributions field.

    Each trade's net PnL is split across its contributing sources in proportion
    to each source's confidence (confidence-weighted attribution). Sources that
    co-fire on winners earn credit; those on losers eat the loss.
    """
    tagged = [t for t in trades if t["contributions"]]
    out = ["## Per-signal attribution\n"]
    if not tagged:
        out.append(
            "_No trades carry the `contributions` field yet — this requires runs "
            "recorded after the 2026-05-29 instrumentation change. Per-signal "
            "realized edge cannot be computed from the current logs (see the "
            "attribution-gap note in PERFORMANCE.md)._\n"
        )
        return "\n".join(out) + "\n"
    by_src = defaultdict(lambda: {"net": 0.0, "weight": 0.0, "n": 0})
    for t in tagged:
        total_conf = sum(t["contributions"].values()) or 1.0
        for src, conf in t["contributions"].items():
            share = conf / total_conf
            by_src[src]["net"] += t["net"] * share
            by_src[src]["weight"] += share
            by_src[src]["n"] += 1
    out.append(
        f"_Confidence-weighted split of net PnL across {len(tagged)} tagged trades._\n"
    )
    out.append("| source | trades | attributed net EUR |")
    out.append("|---|---:|---:|")
    for src, b in sorted(by_src.items(), key=lambda kv: kv[1]["net"]):
        out.append(f"| {src} | {b['n']} | {b['net']:+.4f} |")
    return "\n".join(out) + "\n\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("paths", nargs="*", help="audit JSONL files (default: runs/audit-*.jsonl)")
    ap.add_argument("--taker-fee-pct", type=float, default=0.26)
    ap.add_argument("--report", help="write markdown report to this path")
    args = ap.parse_args()

    paths = args.paths or sorted(glob.glob("runs/audit-*.jsonl"))
    if not paths:
        sys.exit("no audit files found (looked for runs/audit-*.jsonl)")
    taker_fee = args.taker_fee_pct / 100.0

    entries, exits, skips, msg_sources = load_events(paths)
    trades = pair_trades(entries, exits, taker_fee)

    report = []
    report.append("# symm — Performance & Attribution Report\n")
    report.append(
        f"_Generated by `analysis/attribution.py` over {len(paths)} run(s); "
        f"taker fee {args.taker_fee_pct}%/side._\n"
    )
    report.append(summarize(trades))
    report.append(attribution_block(trades))
    report.append(
        group_block(
            "By perspective type",
            lambda t: [PERSPECTIVE_NAMES.get(t["perspective_type"], "unknown")],
            trades,
        )
    )
    report.append(group_block("By symbol", lambda t: [t["symbol"]], trades))
    report.append(
        group_block("By exit reason", lambda t: [t["exit_reason"] or "?"], trades)
    )

    # exit-reason counts across all exits (not just paired)
    exit_reasons = defaultdict(int)
    for x in exits:
        exit_reasons[x.get("reason", "?")] += 1
    report.append("## Exit-reason distribution (all exits)\n")
    for r, c in sorted(exit_reasons.items(), key=lambda kv: -kv[1]):
        report.append(f"- {r}: {c}")
    report.append("")

    # skip reasons
    skip_reasons = defaultdict(int)
    for s in skips:
        skip_reasons[s.get("reason", "?")] += 1
    if skip_reasons:
        report.append("\n## Entry-skip reasons\n")
        for r, c in sorted(skip_reasons.items(), key=lambda kv: -kv[1]):
            report.append(f"- {r}: {c}")
        report.append("")

    # measurement volume per source
    if msg_sources:
        report.append("\n## Measurement volume by source\n")
        total = sum(msg_sources.values())
        for src, c in sorted(msg_sources.items(), key=lambda kv: -kv[1]):
            report.append(f"- {src}: {c} ({100 * c / total:.1f}%)")
        report.append("")

    text = "\n".join(report)
    print(text)
    if args.report:
        os.makedirs(os.path.dirname(args.report) or ".", exist_ok=True)
        with open(args.report, "w") as fh:
            fh.write(text)
        print(f"\n[wrote {args.report}]", file=sys.stderr)


if __name__ == "__main__":
    main()
