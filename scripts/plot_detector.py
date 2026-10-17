#!/usr/bin/env python3
"""Draw the detector tape as price plus the fragments it actually wrote.

A fragment is a flush: the four marks A, B, C, D and the class. The price
line is the tape. Other detector branches stay off the picture. The video
grows the price up to a moving cursor and adds a fragment only once the
cursor reaches the tick where that fragment was written.
"""

import json
import os
import re
import sys

os.environ.setdefault("MPLCONFIGDIR", "/tmp/mpl")

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib import animation

INK = "#d7d0c4"
DIM = "#8a8478"
BG = "#14120f"
GRID = "#2a261f"
CLASS_COLOR = {
    "up": "#7dcea0",
    "down": "#e07a5f",
    "chop": "#e6c15a",
    "flat": "#9aa0a6",
}

SPAN = re.compile(r"tickSpan=(\d+)\s+seqSpan=(\d+)")
BC = re.compile(r"B tick (\d+) seq (\d+) C tick (\d+) seq (\d+)")


def load(path):
    with open(path) as handle:
        doc = json.load(handle)
    return doc["samples"], doc["decisions"]


def why(detail):
    span = SPAN.search(detail or "")
    marks = BC.search(detail or "")
    parts = []
    if span:
        parts.append(f"tickSpan {span.group(1)}")
        parts.append(f"seqSpan {span.group(2)}")
    if marks:
        parts.append(f"B {marks.group(1)} → C {marks.group(3)}")
    return " · ".join(parts)


def fragments(decisions):
    """Pair each flush with the complete that closed it."""
    pending = None
    out = []
    for row in decisions:
        action = row["action"]
        if action == "complete":
            pending = row
            continue
        if action != "flush":
            continue
        closed = pending if pending and pending["symbol"] == row["symbol"] else None
        pending = None
        out.append((row, closed))
    return out


def restarts(samples):
    """Indexes where the workspace tick counter starts over."""
    cuts = []
    for i in range(1, len(samples)):
        if samples[i]["tick"] < samples[i - 1]["tick"]:
            cuts.append(i)
    return cuts


def locate(samples, frags):
    """Place each fragment on the tape, after the previous one.

    A later epoch reuses small tick numbers. Matching the tick from the
    start of the tape would pin that fragment on the earlier epoch.
    """
    cursor = 0
    placed = []
    for flush, closed in frags:
        tick = int(flush["tick"])
        found = None
        for i in range(cursor, len(samples)):
            if int(samples[i]["tick"]) == tick:
                found = i
                break
        if found is None:
            found = cursor
        cursor = found
        placed.append((found, flush, closed))
    return placed


def caption(flush, closed):
    kind = flush.get("detail") or "?"
    tick = int(flush["tick"])
    reason = why(closed["detail"]) if closed else ""
    price = flush.get("price")
    head = f"{kind}  tick {tick}  {price:g}"
    return head if not reason else f"{head}\n{reason}"


def draw(ax, samples, placed, cursor=None):
    ax.set_facecolor(BG)
    shown = samples if cursor is None else samples[: cursor + 1]
    if shown:
        ax.plot(
            list(range(len(shown))),
            [row["price"] for row in shown],
            color=INK,
            linewidth=0.9,
            solid_capstyle="round",
        )
    visible = placed if cursor is None else [(i, f, c) for i, f, c in placed if i <= cursor]
    if shown:
        ymin = min(row["price"] for row in shown)
        ymax = max(row["price"] for row in shown)
        span = ymax - ymin or abs(ymax) * 0.02 or 1.0
        ax.set_ylim(ymin - span * 0.22, ymax + span * 0.38)
        top = ymax + span * 0.30
        step = span * 0.08
        for n, (index, flush, closed) in enumerate(visible):
            color = CLASS_COLOR.get(flush.get("detail") or "", DIM)
            ax.axvline(index, color=color, linewidth=1.1, alpha=0.95)
            ax.text(
                index,
                top - (n % 4) * step,
                caption(flush, closed),
                color=color,
                fontsize=7.5,
                ha="left",
                va="top",
                linespacing=1.25,
            )
        cuts = restarts(shown)
        labels_at = [0, *cuts, len(shown) - 1]
        width = max(len(shown) - 1, 1)
        kept = []
        for index in labels_at:
            if kept and (index - kept[-1]) / width < 0.08:
                continue
            kept.append(index)
        ax.set_xticks(kept)
        ax.set_xticklabels([str(shown[i]["tick"]) for i in kept], rotation=0)
        for cut in restarts(shown):
            ax.axvline(cut, color=DIM, linewidth=0.6, linestyle=":")
            ax.text(
                cut,
                ymax + span * 0.02,
                f"tick restarts at {shown[cut]['tick']}",
                color=DIM,
                fontsize=7,
                ha="left",
                va="bottom",
            )
    ax.set_xlabel("tape, labeled by tick", color=DIM, fontsize=8)
    ax.set_ylabel("price", color=DIM, fontsize=8)
    ax.tick_params(colors=DIM, labelsize=7)
    ax.grid(True, color=GRID, linewidth=0.4)
    for spine in ax.spines.values():
        spine.set_color(GRID)
    latest = visible[-1] if visible else None
    title = samples[0]["symbol"] if samples else ""
    if latest:
        title = f"{title}    {caption(latest[1], latest[2]).replace(chr(10), '  ·  ')}"
    ax.set_title(title, color=INK, fontsize=9, loc="left", pad=8)


def save_symbol(symbol, samples, frags, out_dir):
    safe = symbol.replace("/", "_")
    placed = locate(samples, frags)
    fig, ax = plt.subplots(figsize=(14, 5.2), dpi=140)
    fig.patch.set_facecolor(BG)
    draw(ax, samples, placed)
    png = os.path.join(out_dir, f"{safe}.png")
    fig.savefig(png, facecolor=fig.get_facecolor())
    plt.close(fig)

    if len(samples) > 100:
        stride = max(1, len(samples) // 90)
        cursors = list(range(0, len(samples), stride))
        if cursors[-1] != len(samples) - 1:
            cursors.append(len(samples) - 1)
        for index, _, _ in placed:
            if index not in cursors:
                cursors.append(index)
        cursors = sorted(set(cursors))
    else:
        cursors = list(range(len(samples))) or [0]

    fig, ax = plt.subplots(figsize=(14, 5.2), dpi=110)
    fig.patch.set_facecolor(BG)

    def frame(cursor):
        ax.clear()
        draw(ax, samples, placed, cursor)
        return []

    anim = animation.FuncAnimation(
        fig, frame, frames=cursors, blit=False, interval=180
    )
    mp4 = os.path.join(out_dir, f"{safe}.mp4")
    try:
        anim.save(mp4, writer=animation.FFMpegWriter(fps=5, bitrate=1200))
        movie = mp4
    except (FileNotFoundError, RuntimeError):
        gif = os.path.join(out_dir, f"{safe}.gif")
        anim.save(gif, writer=animation.PillowWriter(fps=4))
        movie = gif
    plt.close(fig)
    print(f"{symbol}: {len(frags)} fragments -> {png} {movie}")


def main():
    if len(sys.argv) != 3:
        print("usage: plot_detector.py TRACE.json OUT_DIR", file=sys.stderr)
        return 2
    samples, decisions = load(sys.argv[1])
    by_symbol = {}
    for row in samples:
        by_symbol.setdefault(row["symbol"], []).append(row)
    frags = {}
    for row, closed in fragments(decisions):
        frags.setdefault(row["symbol"], []).append((row, closed))
    os.makedirs(sys.argv[2], exist_ok=True)
    for symbol in sorted(frags):
        save_symbol(symbol, by_symbol.get(symbol, []), frags[symbol], sys.argv[2])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
