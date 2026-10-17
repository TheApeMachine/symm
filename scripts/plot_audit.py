#!/usr/bin/env python3
"""
SYMM Audit Visualization Engine — Terminal & CRT Aesthetic
Aligned 1:1 with frontend/src/components/ui/theme.css & panel.tsx:
- Panel Header: Full-width docked bar [0, 1], 1px line divider (--line: #2b251e)
- Corners: Crisp rounded-[2px] / rounded-[3px]
- Layout: Left side Title (f1) + Caption (f4/f3 mono); Right side Badge (tone mix) + Readout (f2 mono)
- Zero text collisions: left and right elements are completely separated.
- Scanlines: authentic 3px CRT plate raster overlay matching frontend/src/components/ui/scanlines.tsx
- Margins: ample left margins for y-tick metric names, no clipping.
"""

import os
import sys
import json
import argparse
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.patches import FancyBboxPatch, Rectangle
import numpy as np
from PIL import Image

# Design tokens from frontend theme.css
COLOR_BG = '#0e0c0a'       # --bg
COLOR_SURFACE = '#17140f'  # --surface
COLOR_RAISED = '#1f1a14'   # --raised
COLOR_SUNKEN = '#0a0907'   # --sunken
COLOR_LINE = '#2b251e'     # --line
COLOR_LINE2 = '#3a342b'    # --line2

COLOR_UP = '#9cc06e'       # --up / --success (moss green)
COLOR_DOWN = '#d5786a'     # --down / --error (terracotta coral)
COLOR_WARN = '#e8a33d'     # --warn / --acc (terminal amber)
COLOR_INFO = '#7fbacb'     # --info (slate cyan)

COLOR_F1 = '#f4efe5'       # --f1 (loudest title text)
COLOR_F2 = '#cbc2b4'       # --f2 (body text)
COLOR_F3 = '#938a7e'       # --f3 (muted caption text)
COLOR_F4 = '#5f584e'       # --f4 (dim meta)

plt.style.use('dark_background')
plt.rcParams['font.sans-serif'] = 'Inter Tight, Inter, DejaVu Sans, Arial'
plt.rcParams['font.monospace'] = 'JetBrains Mono, DejaVu Sans Mono, Menlo, monospace'
plt.rcParams['figure.facecolor'] = COLOR_BG
plt.rcParams['axes.facecolor'] = COLOR_SUNKEN
plt.rcParams['axes.edgecolor'] = COLOR_LINE
plt.rcParams['axes.linewidth'] = 1.0
plt.rcParams['grid.color'] = COLOR_LINE
plt.rcParams['grid.linestyle'] = '--'
plt.rcParams['grid.alpha'] = 0.5


def apply_crt_scanlines(image_path, period=3, dark_factor=0.82):
    """
    Applies the CRT plate scanlines overlay matching frontend/src/components/ui/scanlines.tsx:
    `repeating-linear-gradient(0deg, rgba(0,0,0,0.18) 0px, rgba(0,0,0,0.18) 1px, transparent 1px, transparent 3px)`
    """
    if not os.path.exists(image_path):
        return

    img = Image.open(image_path).convert('RGBA')
    arr = np.array(img)
    arr[::period, :, :3] = (arr[::period, :, :3].astype(np.float32) * dark_factor).astype(np.uint8)
    out = Image.fromarray(arr)
    out.save(image_path)


def render_docked_header(fig, title, caption, status="PASS", readout=""):
    """
    Renders a flush full-width docked panel header across [0, 1] in figure coordinates:
    - Left: Title (font-semibold, f1) and Caption (font-mono, f3)
    - Right: Badge (rounded-2px, uppercase mono, tone-mix) and Readout (f2 mono)
    - Bottom divider: 1px solid var(--line)
    - Zero margin from sides: flush dock matching frontend terminal panels.
    - Zero text overlap.
    """
    st = status.upper()
    if any(k in st for k in ['PASS', 'HEALTHY', 'STABLE', 'INDEPENDENT', 'OPTIMAL', 'CLEARED']):
        tone = COLOR_UP
        icon = '●'
    elif any(k in st for k in ['WARN', 'ATTENTION', 'MARGINAL', 'STAGNANT']):
        tone = COLOR_WARN
        icon = '▲'
    elif any(k in st for k in ['FAIL', 'BREACH', 'CRITICAL', 'UNSTABLE']):
        tone = COLOR_DOWN
        icon = '✖'
    else:
        tone = COLOR_INFO
        icon = '◆'

    # 1. Full-bleed docked top bar [0, 0.865] to [1, 1.0]
    bar_rect = Rectangle(
        (0.0, 0.865), 1.0, 0.135,
        facecolor=COLOR_SURFACE,
        edgecolor=None,
        transform=fig.transFigure,
        clip_on=False
    )
    fig.patches.append(bar_rect)

    # 2. Bottom divider line at y=0.865 across full width
    div_line = plt.Line2D(
        [0.0, 1.0], [0.865, 0.865],
        color=COLOR_LINE,
        linewidth=1.2,
        transform=fig.transFigure,
        clip_on=False
    )
    fig.lines.append(div_line)

    # 3. Left Slot: Title & Caption
    fig.text(0.025, 0.945, title, fontsize=12.5, fontweight='600', color=COLOR_F1, va='center')
    fig.text(0.025, 0.902, caption, fontsize=9.0, fontfamily='monospace', color=COLOR_F3, va='center')

    # 4. Right Slot: Badge (rounded 2px, tone color-mix) + Readout
    badge_text = f" {icon} {status} "
    badge_w = 0.12
    badge_x = 0.855
    badge_y = 0.925
    badge_h = 0.038

    # Badge box: rounded 2px
    badge = FancyBboxPatch(
        (badge_x, badge_y), badge_w, badge_h,
        boxstyle='round,pad=0.002,rounding_size=0.0005',
        facecolor=COLOR_RAISED,
        edgecolor=tone,
        linewidth=1.1,
        transform=fig.transFigure,
        clip_on=False
    )
    fig.patches.append(badge)

    fig.text(
        badge_x + badge_w / 2.0, badge_y + badge_h / 2.0,
        badge_text,
        fontsize=8.5, fontweight='bold', fontfamily='monospace',
        color=tone, ha='center', va='center'
    )

    # Metric readout right-aligned directly beneath badge
    if readout:
        fig.text(
            0.975, 0.898,
            readout,
            fontsize=9.0, fontfamily='monospace',
            color=COLOR_F2, ha='right', va='center'
        )


def save_plot(fig, out_path):
    fig.savefig(out_path, dpi=180, facecolor=COLOR_BG)
    plt.close(fig)
    apply_crt_scanlines(out_path)


def plot_stage0_contract(data, out_dir):
    stage = data.get('contract', {})
    breaches = stage.get('breaches', [])
    total_checked = stage.get('total_checked', 0)
    passed = stage.get('passed', len(breaches) == 0)

    fig, ax = plt.subplots(figsize=(13, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.15, right=0.96)

    if passed and len(breaches) == 0:
        ax.text(0.5, 0.5, f"✔ All {total_checked} Observed Metric Contracts Valid\n\nAll signals strictly within declared mathematical domains.\nNo nil pointers or unstandardized entries.",
                ha='center', va='center', fontsize=11.5, color=COLOR_UP, fontfamily='monospace', transform=ax.transAxes,
                bbox=dict(boxstyle='square,pad=1.2', facecolor=COLOR_SURFACE, edgecolor=COLOR_LINE, linewidth=1.0))
        ax.set_xticks([])
        ax.set_yticks([])
        status = "PASS"
        readout = f"{total_checked} METRICS CHECKED  |  0 BREACHES"
    else:
        counts = {}
        for b in breaches:
            name = b.get('name', 'unknown')
            counts[name] = counts.get(name, 0) + 1

        top_names = sorted(counts.keys(), key=lambda k: counts[k], reverse=True)[:10]
        top_counts = [counts[k] for k in top_names]

        y_pos = np.arange(len(top_names))
        bars = ax.barh(y_pos, top_counts, color=COLOR_DOWN, height=0.52, edgecolor=COLOR_LINE)
        ax.set_yticks(y_pos)
        ax.set_yticklabels(top_names, fontsize=9, fontfamily='monospace', color=COLOR_F1)
        ax.invert_yaxis()
        ax.set_xlabel("Contract Breaches Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax.grid(axis='x')

        for bar in bars:
            w = bar.get_width()
            ax.text(w + max(1, w * 0.02), bar.get_y() + bar.get_height() / 2, f"{int(w)}",
                    va='center', fontsize=8.5, fontfamily='monospace', color=COLOR_F1)

        status = "BREACH"
        readout = f"{len(breaches)} BREACHES  |  {total_checked} CHECKED"

    render_docked_header(
        fig,
        f"Stage 0: Metric Contract Integrity ({len(breaches)} Breaches)",
        "Validates hard mathematical domains, causal bounds, and non-nil invariants.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage0_metric_contracts.png"))


def plot_stage1_timing(data, out_dir):
    stage = data.get('timing', {})
    drifts = stage.get('drifts', {})
    total_checked = stage.get('total_checked', 0)
    breach_count = stage.get('breach_count', 0)
    passed = stage.get('passed', breach_count == 0)

    fig, ax = plt.subplots(figsize=(13, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.28, right=0.96)

    if not drifts:
        ax.text(0.5, 0.5, f"✔ Timing Verification Passed ({total_checked} events audited)\nZero feed sequence inversions or venue clock jumps.",
                ha='center', va='center', fontsize=11.5, color=COLOR_UP, fontfamily='monospace', transform=ax.transAxes,
                bbox=dict(boxstyle='square,pad=1.2', facecolor=COLOR_SURFACE, edgecolor=COLOR_LINE, linewidth=1.0))
        ax.set_xticks([])
        ax.set_yticks([])
        status = "PASS"
        readout = f"{total_checked} EVENTS AUDITED  |  0 INVERSIONS"
    else:
        streams = sorted(drifts.keys())[:12]
        clean_labels = [s.replace('|', ' : ') for s in streams]
        means = [drifts[s].get('mean_ms', 0) for s in streams]
        stds = [drifts[s].get('std_ms', 0) for s in streams]

        y_pos = np.arange(len(streams))
        ax.barh(y_pos, means, xerr=stds, color=COLOR_INFO, alpha=0.9, height=0.52,
                edgecolor=COLOR_LINE, error_kw=dict(ecolor=COLOR_F3, lw=1.0, capsize=3))
        ax.set_yticks(y_pos)
        ax.set_yticklabels(clean_labels, fontsize=8.5, fontfamily='monospace', color=COLOR_F1)
        ax.invert_yaxis()
        ax.set_xlabel("Mean Latency Drift ± Jitter Std (ms)", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax.grid(axis='x')

        inversions = stage.get('inversion_count', 0)
        spikes = stage.get('spike_count', 0)
        excess = stage.get('excess_count', 0)
        skew = stage.get('skew_count', 0)
        status = "PASS" if passed else "WARN"
        readout = (
            f"{len(drifts)} STREAMS  |  {inversions} INVERSIONS  |  "
            f"{spikes} SLOW PACKETS  |  {excess} UNSTEADY  |  {skew} CLOCK SKEW"
        )

    render_docked_header(
        fig,
        "Stage 1: Clock Synchronization & Feed Monotonicity",
        "Audits venue-to-ingress transit drift, clock skew, and feed sequence index monotonicity.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage1_feed_timing.png"))


def plot_stage2_vitality(data, out_dir):
    stage = data.get('vitality', {})
    total_metrics = stage.get('catalog_count', stage.get('series_tracked', stage.get('total_metrics', 0)))
    dead_count = stage.get('dead_count', 0)
    unmeasured_count = stage.get('unmeasured_count', 0)
    missing_count = stage.get('missing_count', 0)
    healthy_count = stage.get('healthy_count', max(0, total_metrics - dead_count - unmeasured_count - missing_count))
    active = stage.get('active_symbols') or []

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.18, left=0.08, right=0.96, wspace=0.28)

    categories = ['Moved', 'Missing', 'Always zero', 'Frozen']
    counts = [healthy_count, missing_count, unmeasured_count, dead_count]
    colors = [COLOR_UP, COLOR_WARN, COLOR_DOWN, COLOR_DOWN]

    bars = ax1.bar(categories, counts, color=colors, width=0.52, edgecolor=COLOR_LINE)
    ax1.set_ylabel("Metric Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax1.grid(axis='y')

    for bar in bars:
        h = bar.get_height()
        pct = (h / max(1, total_metrics)) * 100
        ax1.text(bar.get_x() + bar.get_width() / 2, h + max(1, total_metrics * 0.02),
                 f"{int(h)}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=8.5, fontfamily='monospace', color=COLOR_F1)

    top_rows = stage.get('top_variance') or []
    metrics_map = stage.get('metrics', {})
    if top_rows:
        names = [row.get('name', '')[:28] for row in top_rows]
        vars_vals = [row.get('variance', 0) for row in top_rows]
    elif metrics_map:
        top_by_var = sorted(metrics_map.items(), key=lambda kv: kv[1].get('variance', 0), reverse=True)[:10]
        names = [kv[0][:28] for kv in top_by_var]
        vars_vals = [kv[1].get('variance', 0) for kv in top_by_var]
    else:
        names = []
        vars_vals = []

    if names:
        y_pos = np.arange(len(names))
        ax2.barh(y_pos, vars_vals, color=COLOR_INFO, height=0.52, edgecolor=COLOR_LINE)
        ax2.set_yticks(y_pos)
        ax2.set_yticklabels(names, fontsize=8.5, fontfamily='monospace', color=COLOR_F1)
        ax2.invert_yaxis()
        ax2.set_xlabel("Sample Variance", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax2.set_title("Top 10 Metrics by Variance", fontsize=10, fontfamily='monospace', color=COLOR_F2)
        ax2.grid(axis='x')
    else:
        ax2.text(0.5, 0.5, "No individual metric variance available", ha='center', va='center', color=COLOR_F3)

    passed = stage.get('passed', dead_count == 0 and unmeasured_count == 0 and missing_count == 0)
    status = "HEALTHY" if passed else "ATTENTION"
    shown = ", ".join(active) if active else "NO ACTIVE SYMBOLS"
    readout = (
        f"{shown}  |  {healthy_count}/{total_metrics} MOVED  |  "
        f"{missing_count} MISSING  |  {unmeasured_count} ALWAYS ZERO  |  {dead_count} FROZEN"
    )

    render_docked_header(
        fig,
        "Stage 2: Do the signals in SIGNALS.md move?",
        "A few symbols whose price moved. Each listed signal has to change on those symbols.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage2_metric_vitality.png"))


def plot_stage3_redundancy(data, out_dir):
    stage = data.get('redundancy', {})
    pairs = stage.get('pairs', [])
    total_pairs = stage.get('pairs_evaluated', stage.get('total_pairs', len(pairs)))
    redundant_pairs = stage.get('redundant_pairs') or [p for p in pairs if p.get('collinear')]
    redundant_count = stage.get('collinear_count', stage.get('redundant_count', len(redundant_pairs)))
    passed = stage.get('passed', redundant_count == 0)

    fig, ax = plt.subplots(figsize=(13, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.34, right=0.96)

    if not redundant_pairs:
        ax.text(0.5, 0.5, f"✔ Orthogonal Representation Space\n\nAudited {total_pairs} metric pairs: zero collinear clones (|r| >= 0.95).\nEvery active dimension contributes unique information.",
                ha='center', va='center', fontsize=11.5, color=COLOR_UP, fontfamily='monospace', transform=ax.transAxes,
                bbox=dict(boxstyle='square,pad=1.2', facecolor=COLOR_SURFACE, edgecolor=COLOR_LINE, linewidth=1.0))
        ax.set_xticks([])
        ax.set_yticks([])
        status = "INDEPENDENT"
        readout = f"{total_pairs} PAIRS EVALUATED  |  0 CLONES"
    else:
        top_pairs = redundant_pairs[:12]
        clean_labels = [p.get('pair', 'pair').replace('|', ' : ') for p in top_pairs]
        corrs = [abs(float(p.get('correlation', 0.0))) for p in top_pairs]

        y_pos = np.arange(len(clean_labels))
        bars = ax.barh(y_pos, corrs, color=COLOR_WARN, height=0.52, edgecolor=COLOR_LINE)
        ax.set_yticks(y_pos)
        ax.set_yticklabels(clean_labels, fontsize=8.5, fontfamily='monospace', color=COLOR_F1)
        ax.invert_yaxis()
        ax.set_xlim(0.90, 1.01)
        ax.set_xlabel("Pearson Correlation (|r|)", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax.grid(axis='x')

        for bar in bars:
            w = bar.get_width()
            ax.text(w - 0.008, bar.get_y() + bar.get_height() / 2, f"{w:.3f}",
                    va='center', ha='right', fontsize=8, fontfamily='monospace', color=COLOR_BG, fontweight='bold')

        status = "WARN"
        readout = f"{redundant_count} COLLINEAR PAIRS  |  {total_pairs} EVALUATED"

    render_docked_header(
        fig,
        "Stage 3: Feature Orthogonality & Collinear Redundancy",
        "Audits pairwise correlation to detect duplicate sensors wasting capacity or distorting risk models.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage3_representation_redundancy.png"))


def plot_stage4_sympathy(data, out_dir):
    stage = data.get('sympathy', {})
    total_streams = stage.get('total_streams', 0)
    estimates = stage.get('estimates', [])
    redundant_count = stage.get('redundant_count', 0)

    fig, ax = plt.subplots(figsize=(13, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.34, right=0.96)

    if not estimates:
        ax.text(0.5, 0.5, f"✔ Asynchronous Continuous-Time Sympathy Evaluated\n\nTracked {total_streams} asynchronous timelines.\nNo cross-source redundant clone streams detected via Hayashi-Yoshida.",
                ha='center', va='center', fontsize=11.5, color=COLOR_UP, fontfamily='monospace', transform=ax.transAxes,
                bbox=dict(boxstyle='square,pad=1.2', facecolor=COLOR_SURFACE, edgecolor=COLOR_LINE, linewidth=1.0))
        ax.set_xticks([])
        ax.set_yticks([])
        status = "INDEPENDENT"
        readout = f"{total_streams} TIMELINES  |  0 CLONES"
    else:
        top_est = sorted(estimates, key=lambda e: abs(e.get('concordance', 0)), reverse=True)[:10]
        clean_labels = [e.get('pair', '').replace('|', ' : ') for e in top_est]
        concs = [e.get('concordance', 0) for e in top_est]

        y_pos = np.arange(len(clean_labels))
        colors = [COLOR_DOWN if abs(c) >= 0.95 else COLOR_INFO for c in concs]
        ax.barh(y_pos, concs, color=colors, height=0.52, edgecolor=COLOR_LINE)
        ax.set_yticks(y_pos)
        ax.set_yticklabels(clean_labels, fontsize=8.5, fontfamily='monospace', color=COLOR_F1)
        ax.invert_yaxis()
        ax.set_xlim(-1.05, 1.05)
        ax.axvline(0, color=COLOR_LINE2, linestyle='--')
        ax.set_xlabel("Hayashi-Yoshida Overlap Concordance [-1, 1]", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax.grid(axis='x')

        status = "WARN" if redundant_count > 0 else "PASS"
        readout = f"{len(estimates)} PAIRS EVALUATED  |  {redundant_count} REDUNDANT"

    render_docked_header(
        fig,
        "Stage 4: Asynchronous Cross-Stream Sympathy",
        "Continuous-time Hayashi-Yoshida overlap covariance without artificial time bucketing (no Epps distortion).",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage4_asynchronous_sympathy.png"))


def plot_stage5_stability(data, out_dir):
    stage = data.get('stability', {})
    total_frames = stage.get('total_frames', 0)
    valid_frames = stage.get('valid_frames', 0)
    completeness = stage.get('completeness', 0)
    jsd = stage.get('jsd', 0)
    tvd = stage.get('tvd', 0)
    passed = stage.get('passed', False)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.08, right=0.96, wspace=0.28)

    comp_pct = completeness * 100
    missing = max(0, total_frames - valid_frames)
    ax1.bar(['Observed\nFrames', 'Empty\nFrames'], [valid_frames, missing],
            color=[COLOR_UP, COLOR_DOWN if missing > 0 else COLOR_LINE], width=0.48, edgecolor=COLOR_LINE)
    ax1.set_ylabel("Frame Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax1.set_title(f"Grid Completeness: {comp_pct:.1f}% ({valid_frames}/{total_frames})", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax1.grid(axis='y')

    ax2.bar(['Empirical JSD', 'Stationarity Bound'], [jsd, 0.40],
            color=[COLOR_DOWN if jsd > 0.40 else COLOR_INFO, COLOR_LINE2], width=0.45, edgecolor=COLOR_LINE)
    ax2.set_ylabel("JSD (Bits)", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax2.set_title(f"Regime Divergence: JSD = {jsd:.3f}b (TVD = {tvd:.3f})", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax2.grid(axis='y')

    status = "STABLE" if passed else "UNSTABLE"
    readout = f"COMPLETENESS: {comp_pct:.1f}%  |  JSD: {jsd:.3f}b"

    render_docked_header(
        fig,
        "Stage 5: Feature Grid Stationarity & Completeness",
        "Audits frame excitation completeness and regime shift stability via Jensen-Shannon divergence.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage5_grid_stability.png"))


def plot_stage6_dynamics(data, out_dir):
    stage = data.get('dynamics', {})
    entropy = stage.get('entropy', 0)
    max_entropy = stage.get('max_entropy', 0)
    total_tokens = stage.get('total_tokens', 0)
    distinct_tokens = stage.get('distinct_tokens', 0)
    token_counts = stage.get('token_counts', {})
    passed = stage.get('passed', False)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.08, right=0.96, wspace=0.28)

    if token_counts:
        top_tokens = sorted(token_counts.items(), key=lambda kv: kv[1], reverse=True)[:8]
        names = [kv[0][:18] for kv in top_tokens]
        counts = [kv[1] for kv in top_tokens]

        y_pos = np.arange(len(names))
        ax1.barh(y_pos, counts, color=COLOR_INFO, height=0.52, edgecolor=COLOR_LINE)
        ax1.set_yticks(y_pos)
        ax1.set_yticklabels(names, fontsize=8.5, fontfamily='monospace', color=COLOR_F1)
        ax1.invert_yaxis()
        ax1.set_xlabel("Emissions Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax1.set_title(f"State Occupancy ({distinct_tokens} States)", fontsize=10, fontfamily='monospace', color=COLOR_F2)
        ax1.grid(axis='x')
    else:
        ax1.text(0.5, 0.5, "No token emissions", ha='center', va='center', color=COLOR_F3)

    ax2.bar(['Empirical H(S)', 'Max Random H'], [entropy, max(0.01, max_entropy)],
            color=[COLOR_UP, COLOR_LINE2], width=0.45, edgecolor=COLOR_LINE)
    ax2.set_ylabel("Entropy (Bits)", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    structure_pct = (1.0 - (entropy / max(1e-6, max_entropy))) * 100 if max_entropy > 0 else 0
    ax2.set_title(f"Predictive Structure: {structure_pct:.1f}% Organized", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax2.grid(axis='y')

    status = "STRUCTURED" if passed else "ATTENTION"
    readout = f"H = {entropy:.3f}b / {max_entropy:.3f}b  |  {distinct_tokens} STATES"

    render_docked_header(
        fig,
        "Stage 6: Discrete Token Dynamics & Transition Entropy",
        "Audits sequential state transition entropy to differentiate predictive structure from random walk noise.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage6_token_dynamics.png"))


def plot_stage7_precursor(data, out_dir):
    stage = data.get('precursor', {})
    excursion_count = stage.get('excursion_count', 0)
    clearance_rate = stage.get('clearance_rate', 0)
    mean_net_return = stage.get('mean_net_return', 0)
    taker_fee = stage.get('taker_fee', 0.0)
    excursions = stage.get('excursions', [])
    passed = stage.get('passed', False)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.08, right=0.96, wspace=0.28)

    prof_count = int(round(clearance_rate * excursion_count))
    unprof_count = excursion_count - prof_count
    ax1.bar(['Profitable\n(Net > 0)', 'Fee/Spread\nEaten'], [prof_count, unprof_count],
            color=[COLOR_UP, COLOR_DOWN if unprof_count > 0 else COLOR_LINE], width=0.48, edgecolor=COLOR_LINE)
    ax1.set_ylabel("Excursion Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax1.set_title(f"Friction Clearance: {clearance_rate*100:.1f}% ({prof_count}/{excursion_count})", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax1.grid(axis='y')

    if excursions:
        returns = [e.get('net_return', 0) * 100 for e in excursions[:30]]
        x_pos = np.arange(len(returns))
        bar_colors = [COLOR_UP if r > 0 else COLOR_DOWN for r in returns]
        ax2.bar(x_pos, returns, color=bar_colors, width=0.55, edgecolor=COLOR_LINE)
        ax2.axhline(0, color=COLOR_F3, linestyle='--', linewidth=0.8)
        ax2.set_xlabel("Excursion Index", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax2.set_ylabel("Net Return % (Post Fee)", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax2.set_title(f"Mean Net Return: {mean_net_return*100:+.2f}%", fontsize=10, fontfamily='monospace', color=COLOR_F2)
        ax2.grid(axis='y')
    else:
        ax2.text(0.5, 0.5, "No completed order book touch excursions", ha='center', va='center', color=COLOR_F3)

    status = "CLEARED" if passed else "BREACH"
    readout = f"CLEARANCE: {clearance_rate*100:.1f}%  |  MEAN: {mean_net_return*100:+.2f}%"

    render_docked_header(
        fig,
        "Stage 7: Precursor Excursions & Order Book Friction",
        "Audits execution viability: crossing Ask on entry, Bid on exit, after full round-trip taker trading fees.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage7_precursor_excursions.png"))


def plot_stage8_cognitive(data, out_dir):
    stage = data.get('cognitive', {})
    total_phases = stage.get('total_phases', 0)
    wait_count = stage.get('wait_count', 0)
    enter_count = stage.get('enter_count', 0)
    exit_count = stage.get('exit_count', 0)
    abstention_rate = stage.get('abstention_rate', 0)
    mean_confidence = stage.get('mean_confidence', 0)
    passed = stage.get('passed', False)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.08, right=0.96, wspace=0.28)

    categories = ['Wait\n(Abstain)', 'Enter\n(Risk)', 'Exit\n(De-risk)']
    counts = [wait_count, enter_count, exit_count]
    colors = [COLOR_INFO, COLOR_UP, COLOR_WARN]

    bars = ax1.bar(categories, counts, color=colors, width=0.52, edgecolor=COLOR_LINE)
    ax1.set_ylabel("Decision Cycles", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax1.set_title(f"Decisiveness: {(1-abstention_rate)*100:.1f}% Active", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax1.grid(axis='y')

    for bar in bars:
        h = bar.get_height()
        pct = (h / max(1, total_phases)) * 100
        ax1.text(bar.get_x() + bar.get_width() / 2, h + max(1, total_phases * 0.02),
                 f"{int(h)}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=8.5, fontfamily='monospace', color=COLOR_F1)

    ax2.bar(['Mean Confidence', 'Active Rate'], [mean_confidence, 1.0 - abstention_rate],
            color=[COLOR_INFO, COLOR_UP], width=0.45, edgecolor=COLOR_LINE)
    ax2.set_ylabel("Score / Ratio [0, 1]", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax2.set_title(f"Confidence Calibration: {mean_confidence:.3f}", fontsize=10, fontfamily='monospace', color=COLOR_F2)
    ax2.set_ylim(0, 1.05)
    ax2.grid(axis='y')

    status = "OPTIMAL" if passed else "ATTENTION"
    readout = f"PHASES: {total_phases}  |  ABSTAIN: {abstention_rate*100:.1f}%  |  CONF: {mean_confidence:.3f}"

    render_docked_header(
        fig,
        "Stage 8: Cognitive Decision Engine & Memory Dynamics",
        "Audits decision forming, confidence calibration, and guarantees the engine does not freeze in total abstention.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage8_cognitive_learning.png"))


def plot_stage9_truthfulness(data, out_dir):
    stage = data.get('truthfulness', {})
    total_checked = stage.get('total_checked', 0)
    crossed_count = stage.get('crossed_market_count', 0)
    vol_div_count = stage.get('volume_divergence_count', 0)
    breach_count = stage.get('breach_count', 0)
    passed = stage.get('passed', breach_count == 0)

    fig, ax = plt.subplots(figsize=(13, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.28, right=0.96)

    if passed and breach_count == 0:
        ax.text(0.5, 0.5, f"✔ Tape Truthfulness & Physical Identities Confirmed\n\nAudited {total_checked} observations:\n• Zero crossed markets (Best Bid <= Best Ask maintained throughout)\n• Zero volume conservation divergences (Gross == Buy + Sell maintained).",
                ha='center', va='center', fontsize=11.5, color=COLOR_UP, fontfamily='monospace', transform=ax.transAxes,
                bbox=dict(boxstyle='square,pad=1.2', facecolor=COLOR_SURFACE, edgecolor=COLOR_LINE, linewidth=1.0))
        ax.set_xticks([])
        ax.set_yticks([])
        status = "PASS"
        readout = f"{total_checked} VERIFIED  |  0 IDENTITY BREACHES"
    else:
        labels = ['Crossed Quotes (Bid > Ask)', 'Volume Conservation Divergence']
        counts = [crossed_count, vol_div_count]
        y_pos = np.arange(len(labels))
        ax.barh(y_pos, counts, color=COLOR_DOWN, height=0.48, edgecolor=COLOR_LINE)
        ax.set_yticks(y_pos)
        ax.set_yticklabels(labels, fontsize=9, fontfamily='monospace', color=COLOR_F1)
        ax.invert_yaxis()
        ax.set_xlabel("Number of Identity Violations", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
        ax.grid(axis='x')

        status = "BREACH"
        readout = f"{breach_count} IDENTITY BREACHES ({crossed_count} CROSSED)"

    render_docked_header(
        fig,
        "Stage 9: Tape Truthfulness & Accounting Identities",
        "Audits physical market identities: non-crossed order books and exact trade volume conservation.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage9_tape_truthfulness.png"))


def plot_summary_dashboard(data, out_dir):
    stages = [
        ("Stage 0 : Contract", data.get('contract', {}).get('passed', False)),
        ("Stage 1 : Timing", data.get('timing', {}).get('passed', False)),
        ("Stage 2 : Vitality", data.get('vitality', {}).get('passed', False)),
        ("Stage 3 : Redundancy", data.get('redundancy', {}).get('passed', False)),
        ("Stage 4 : Sympathy", data.get('sympathy', {}).get('passed', False)),
        ("Stage 5 : Stability", data.get('stability', {}).get('passed', False)),
        ("Stage 6 : Dynamics", data.get('dynamics', {}).get('passed', False)),
        ("Stage 7 : Precursor", data.get('precursor', {}).get('passed', False)),
        ("Stage 8 : Cognitive", data.get('cognitive', {}).get('passed', False)),
        ("Stage 9 : Truthfulness", data.get('truthfulness', {}).get('passed', False)),
    ]

    fig, ax = plt.subplots(figsize=(14, 7.5))
    fig.subplots_adjust(top=0.80, bottom=0.10, left=0.25, right=0.92)

    names = [s[0] for s in stages]
    passed_vals = [1 if s[1] else 0 for s in stages]
    colors = [COLOR_UP if s[1] else COLOR_DOWN for s in stages]

    y_pos = np.arange(len(names))
    bars = ax.barh(y_pos, [1] * len(names), color=colors, height=0.52, edgecolor=COLOR_LINE)
    ax.set_yticks(y_pos)
    ax.set_yticklabels(names, fontsize=9.5, fontfamily='monospace', color=COLOR_F1)
    ax.invert_yaxis()
    ax.set_xlim(0, 1.15)
    ax.set_xticks([])

    for i, bar in enumerate(bars):
        txt = "PASS / OPTIMAL" if stages[i][1] else "BREACH / ATTENTION"
        c = COLOR_UP if stages[i][1] else COLOR_DOWN
        ax.text(1.02, bar.get_y() + bar.get_height() / 2, txt,
                va='center', fontsize=9, fontweight='bold', fontfamily='monospace', color=c)

    pass_count = sum(passed_vals)
    health_score = (pass_count / len(stages)) * 100
    status = "OPTIMAL" if pass_count == 10 else ("ATTENTION" if pass_count >= 7 else "CRITICAL")
    readout = f"SCORE: {pass_count}/10 STAGES ({health_score:.0f}%)"

    render_docked_header(
        fig,
        "SYMM Empirical Health Audit Executive Summary",
        "Multi-dimensional verification across contract, timing, vitality, redundancy, sympathy, stability, dynamics, precursor, cognitive, and truthfulness.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "audit_summary_scorecard.png"))


# -------------------------------------------------------------------------
# Exact Nevill-Manning & Witten (1997) Sequitur Grammar Engine
# -------------------------------------------------------------------------
class SequiturSymbol:
    def __init__(self, value=None, rule=None, is_guard=False):
        self.value = value       # Terminal string e.g. "R00"
        self.rule = rule         # Non-terminal Rule reference
        self.is_guard = is_guard
        self.prev = self
        self.next = self
        self.owner = None

    def name(self):
        if self.is_guard:
            return "^"
        if self.rule is not None:
            return f"R{self.rule.id}"
        return str(self.value)

    def key(self):
        if self.is_guard:
            return ("guard", 0)
        if self.rule is not None:
            return ("rule", self.rule.id)
        return ("term", self.value)


class SequiturRule:
    def __init__(self, rule_id):
        self.id = rule_id
        self.guard = SequiturSymbol(is_guard=True)
        self.guard.prev = self.guard
        self.guard.next = self.guard
        self.guard.owner = self
        self.count = 0

    def length(self):
        cnt = 0
        curr = self.guard.next
        while curr != self.guard and curr is not None:
            cnt += 1
            curr = curr.next
        return cnt

    def depth(self):
        max_d = 0
        curr = self.guard.next
        while curr != self.guard and curr is not None:
            if curr.rule is not None:
                d = curr.rule.depth()
                if d > max_d:
                    max_d = d
            curr = curr.next
        return max_d + 1

    def symbols_str(self):
        parts = []
        curr = self.guard.next
        while curr != self.guard and curr is not None:
            parts.append(curr.name())
            curr = curr.next
        return parts


class SequiturGrammar:
    def __init__(self):
        self.rules = {}
        self.digrams = {}
        self.next_rule_id = 1
        self.start_rule = SequiturRule(0)
        self.rules[0] = self.start_rule

    def _join(self, left, right):
        if left.next is not None:
            self._delete_digram(left)
        if right.prev is not None and right.prev != left:
            self._delete_digram(right.prev)
        left.next = right
        right.prev = left

    def _digram_key(self, sym):
        if sym is None or sym.next is None or sym.is_guard or sym.next.is_guard:
            return None
        return (sym.key(), sym.next.key())

    def _delete_digram(self, sym):
        if sym.is_guard or sym.next is None or sym.next.is_guard:
            return
        k = self._digram_key(sym)
        if k in self.digrams and self.digrams[k] == sym:
            del self.digrams[k]

    def append(self, term_val):
        new_sym = SequiturSymbol(value=term_val)
        new_sym.owner = self.start_rule
        last = self.start_rule.guard.prev
        self._join(last, new_sym)
        self._join(new_sym, self.start_rule.guard)
        self._check_digram(last)

        active = self.start_rule.guard.prev
        if active and not active.is_guard and active.rule:
            return active.rule
        return None

    def _check_digram(self, sym):
        if sym.is_guard or sym.next is None or sym.next.is_guard:
            return

        k = self._digram_key(sym)
        if k is None:
            return

        match = self.digrams.get(k)
        if match is None:
            self.digrams[k] = sym
            return

        if match.next == sym:
            return

        rule = None
        if match.prev.is_guard and match.next.next.is_guard:
            rule = match.owner
            self._substitute(sym, rule)
        else:
            rule = SequiturRule(self.next_rule_id)
            self.next_rule_id += 1
            self.rules[rule.id] = rule

            first = SequiturSymbol(value=sym.value, rule=sym.rule)
            first.owner = rule
            second = SequiturSymbol(value=sym.next.value, rule=sym.next.rule)
            second.owner = rule

            self._join(rule.guard, first)
            self._join(first, second)
            self._join(second, rule.guard)

            self._substitute(match, rule)
            self._substitute(sym, rule)

            self.digrams[self._digram_key(first)] = first

        if rule and rule.id != 0:
            first = rule.guard.next
            if first and first.rule and first.rule.count == 1:
                self._expand(first)
            second = rule.guard.next.next
            if second and second != rule.guard and second.rule and second.rule.count == 1:
                self._expand(second)

    def _substitute(self, sym, rule):
        prev = sym.prev
        next_after = sym.next.next

        self._delete_digram(sym.prev)
        self._delete_digram(sym)
        self._delete_digram(sym.next)

        if sym.rule:
            sym.rule.count -= 1
        if sym.next.rule:
            sym.next.rule.count -= 1

        new_sym = SequiturSymbol(rule=rule)
        new_sym.owner = sym.owner
        rule.count += 1

        self._join(prev, new_sym)
        self._join(new_sym, next_after)

        self._check_digram(new_sym.prev)
        self._check_digram(new_sym)

    def _expand(self, sym):
        rule = sym.rule
        if rule is None or rule.id == 0 or rule.count != 1:
            return

        prev = sym.prev
        next_node = sym.next

        self._delete_digram(prev)
        self._delete_digram(sym)

        first = rule.guard.next
        last = rule.guard.prev

        curr = first
        while curr != rule.guard:
            curr.owner = sym.owner
            curr = curr.next

        self._join(prev, first)
        self._join(last, next_node)

        del self.rules[rule.id]

        self._check_digram(prev)
        self._check_digram(last)

    def num_rules(self):
        return len(self.rules) - 1

    def total_grammar_size(self):
        tot = 0
        for r in self.rules.values():
            tot += r.length()
        return tot

    def compression_ratio(self, raw_tokens_count):
        g_size = self.total_grammar_size()
        if g_size <= 0:
            return 1.0
        return raw_tokens_count / g_size


def animate_excursion_transitions(data, out_dir):
    """
    Renders the dynamic multi-panel animation of regional confluence transitions,
    excursion return trajectory with A, B, C markers and enter/exit predictions,
    and Sequitur grammar induction timeline over market tape in frontend-aligned CRT styling.
    Produces:
    - excursion_transitions.mp4 (H.264 via ffmpeg)
    - excursion_transitions.gif (Pillow animated raster)
    - excursion_transitions_summary.png (static companion sheet with scanline CRT overlay)
    """
    import matplotlib.animation as anim
    import shutil

    dynamics_data = data.get('dynamics', {})
    frames = dynamics_data.get('frames', [])
    precursor_data = data.get('precursor', {})
    episodes = precursor_data.get('episodes', [])
    cognitive_data = data.get('cognitive', {})
    phases = cognitive_data.get('phases', [])

    if not frames:
        raise ValueError(
            "[plot_audit] Cannot render excursion transitions animation: "
            "No empirical frames found in dynamics audit data. "
            "Synthetic fallbacks and approximations are strictly forbidden."
        )

    regions_all = [f"R{i:02d}" for i in range(15)]

    # Subsample frames evenly if sequence is too long for video rendering (> 120 steps)
    max_anim_steps = 120
    if len(frames) > max_anim_steps:
        indices = np.linspace(0, len(frames) - 1, max_anim_steps, dtype=int)
        sampled_frames = [frames[idx] for idx in indices]
    else:
        sampled_frames = frames

    total_steps = len(sampled_frames)

    # Extract strictly empirical profiles, active regions, returns, and prices
    step_profiles = []
    cum_returns = []
    prices = []
    ticks = []
    active_tokens = []

    for frame in sampled_frames:
        active_tok = frame.get('active_token', 'R00')
        active_tokens.append(active_tok)
        active_idx = int(active_tok[1:]) if (active_tok.startswith('R') and active_tok[1:].isdigit()) else 0

        brightness_raw = frame.get('brightness', [])
        profile = np.zeros(15)
        for r_i in range(min(15, len(brightness_raw))):
            profile[r_i] = float(brightness_raw[r_i])

        sorted_indices = np.argsort(profile)[::-1]
        ru_idx = sorted_indices[1] if len(sorted_indices) > 1 and profile[sorted_indices[1]] > 0 else -1

        step_profiles.append((active_idx, ru_idx, profile))
        cum_returns.append(float(frame.get('return', 0.0)))
        prices.append(float(frame.get('price', 0.0)))
        ticks.append(frame.get('tick', 0))

    # 1. Execute Sequitur Grammar Engine across empirical token stream
    g = SequiturGrammar()
    rules_hist = []
    comp_hist = []
    active_rules_str = []
    rule_events = []

    for i, tok in enumerate(active_tokens):
        rule = g.append(tok)
        nr = g.num_rules()
        cr = g.compression_ratio(i + 1)
        rules_hist.append(nr)
        comp_hist.append(cr)
        r_str = f"R{rule.id} -> [{' '.join(rule.symbols_str())}]" if rule else ""
        active_rules_str.append(r_str)
        if rule and (len(rule_events) == 0 or rule_events[-1][1] != f"R{rule.id}"):
            rule_events.append((i, f"R{rule.id}", r_str))

    # 2. Excursion Episode Coordinates (A, B, C)
    # Pick the most prominent upward excursion from empirical detector episodes
    ep = None
    for item in episodes:
        if item.get('class') in ['up', 'up_friction'] and item.get('c_tick', 0) > item.get('b_tick', 0):
            if ep is None or (item.get('c_price', 0) - item.get('b_price', 0)) > (ep.get('c_price', 0) - ep.get('b_price', 0)):
                ep = item

    mark_A = annot_A = mark_B = annot_B = mark_C = annot_C = None
    idx_A = idx_B = idx_C = 0

    if ep:
        s_tick = ep['start_tick']
        b_tick = ep['b_tick']
        c_tick = ep['c_tick']
        b_price = ep['b_price']
        c_price = ep['c_price']

        idx_A = min(range(len(ticks)), key=lambda i: abs(ticks[i] - s_tick))
        idx_B = min(range(len(ticks)), key=lambda i: abs(ticks[i] - b_tick))
        idx_C = min(range(len(ticks)), key=lambda i: abs(ticks[i] - c_tick))

        y_A = cum_returns[idx_A]
        y_B = cum_returns[idx_B]
        y_C = cum_returns[idx_C]

    # Setup 3-panel figure: Left = Confluence Bars (15 regions), Right Top = Excursion Trajectory, Right Bottom = Sequitur State
    fig = plt.figure(figsize=(15.5, 8.8))
    gs = fig.add_gridspec(2, 2, width_ratios=[1.05, 1.45], height_ratios=[1.0, 1.0],
                           left=0.08, right=0.94, top=0.83, bottom=0.10, wspace=0.28, hspace=0.36)

    ax_bars = fig.add_subplot(gs[:, 0])
    ax_traj = fig.add_subplot(gs[0, 1])
    ax_seq = fig.add_subplot(gs[1, 1])

    # Left: Confluence Bars
    y_pos = np.arange(15)
    init_brightness = np.zeros(15)
    bars = ax_bars.barh(y_pos, init_brightness, height=0.58, color=COLOR_RAISED, edgecolor=COLOR_LINE)
    ax_bars.set_yticks(y_pos)
    ax_bars.set_yticklabels(regions_all, fontsize=9, fontfamily='monospace', color=COLOR_F2)
    ax_bars.invert_yaxis()
    max_b = max([p[2].max() for p in step_profiles]) if step_profiles else 1.0
    x_limit = max(1.8, max_b * 1.15)
    ax_bars.set_xlim(0, x_limit)
    step_tick = 1 if x_limit <= 6 else 2
    ax_bars.set_xticks(np.arange(0, int(x_limit) + step_tick, step_tick))
    ax_bars.set_xlabel("Region Brightness |SNM|", fontsize=9.0, fontfamily='monospace', color=COLOR_F3)
    ax_bars.set_title("Multi-Signal Confluence (R00 - R14)", fontsize=10.5, fontfamily='monospace', color=COLOR_F1)
    ax_bars.grid(axis='x', linestyle='--', alpha=0.4)

    bar_labels = []
    for y in y_pos:
        txt = ax_bars.text(0.02, y, "", va='center', fontsize=8.5, fontfamily='monospace', color=COLOR_F3)
        bar_labels.append(txt)

    # Right Top: Trajectory with A B C Markers & Enter/Exit Predictions
    min_ret = min(cum_returns)
    max_ret = max(cum_returns)
    pad = max(0.10, (max_ret - min_ret) * 0.45)
    ax_traj.set_xlim(0, total_steps - 1)
    ax_traj.set_ylim(min_ret - pad, max_ret + pad * 1.35)
    ax_traj.axhline(0, color=COLOR_F4, linestyle='--', linewidth=0.8)
    ax_traj.set_ylabel("Net Return %", fontsize=9.0, fontfamily='monospace', color=COLOR_F3)
    ax_traj.set_title("Excursion Trajectory & Cognitive Policy Predictions", fontsize=10.5, fontfamily='monospace', color=COLOR_F1)
    ax_traj.grid(True, linestyle='--', alpha=0.3)

    trail_line, = ax_traj.plot([], [], color=COLOR_INFO, linewidth=2.0, zorder=2)
    head_dot, = ax_traj.plot([], [], marker='o', markersize=8, color=COLOR_UP, markeredgecolor=COLOR_F1, markeredgewidth=1.5, zorder=6)
    cursor_traj = ax_traj.axvline(0, color=COLOR_DOWN, linestyle=':', linewidth=1.2, alpha=0.8)

    if ep:
        # Marker A: Precursor Base
        mark_A, = ax_traj.plot(idx_A, y_A, marker='D', markersize=9, color=COLOR_INFO, markeredgecolor=COLOR_F1, markeredgewidth=1.5, zorder=5)
        annot_A = ax_traj.annotate(
            f"[A] PRECURSOR BASE\nTick {ticks[idx_A]} | ${prices[idx_A]:,.1f}",
            xy=(idx_A, y_A), xytext=(idx_A - 8, y_A - 0.055),
            fontsize=8.0, fontfamily='monospace', color=COLOR_INFO, fontweight='bold',
            bbox=dict(boxstyle='round,pad=0.25', facecolor=COLOR_RAISED, edgecolor=COLOR_INFO, alpha=0.9),
            arrowprops=dict(arrowstyle='->', color=COLOR_INFO, lw=1.0)
        )

        # Marker B: Ignition Anchor & Cognitive Entry Evaluation
        mark_B, = ax_traj.plot(idx_B, y_B, marker='^', markersize=11, color=COLOR_WARN, markeredgecolor=COLOR_F1, markeredgewidth=1.5, zorder=5)
        if phases:
            b_phase = phases[min(idx_B, len(phases) - 1)]
            b_action = b_phase.get('action', 'WAIT').upper()
            b_conf = b_phase.get('confidence', 0.0)
            b_cog_str = f"COGNITIVE: {b_action} (conf: {b_conf:.2f})"
        else:
            b_cog_str = "COGNITIVE: NO DECISIONS"
        annot_B = ax_traj.annotate(
            f"[B] IGNITION ANCHOR\nTick {ticks[idx_B]} | ${b_price:,.1f}\n{b_cog_str}",
            xy=(idx_B, y_B), xytext=(idx_B - 14, y_B + 0.065),
            fontsize=8.0, fontfamily='monospace', color=COLOR_WARN, fontweight='bold',
            bbox=dict(boxstyle='round,pad=0.3', facecolor=COLOR_RAISED, edgecolor=COLOR_WARN, alpha=0.95),
            arrowprops=dict(arrowstyle='->', color=COLOR_WARN, lw=1.2)
        )

        # Marker C: Retrace Pivot / Bounce Apex (Friction Clearance Assessment)
        mark_C, = ax_traj.plot(idx_C, y_C, marker='o', markersize=10, color=COLOR_UP, markeredgecolor=COLOR_F1, markeredgewidth=1.5, zorder=5)
        ep_class = ep.get('class', 'up')
        c_status = "BOUNCE APEX (FRICTION)" if ep_class == 'up_friction' else "CONFIRMED ZIGZAG PIVOT"
        gross_move = ((c_price - b_price) / b_price * 100.0) if b_price > 0 else 0.0
        annot_C = ax_traj.annotate(
            f"[C] {c_status}\nTick {ticks[idx_C]} | ${c_price:,.1f}\nGross Move: {gross_move:+.2f}%",
            xy=(idx_C, y_C), xytext=(idx_C + 4, y_C + 0.04),
            fontsize=8.0, fontfamily='monospace', color=COLOR_UP, fontweight='bold',
            bbox=dict(boxstyle='round,pad=0.3', facecolor=COLOR_RAISED, edgecolor=COLOR_UP, alpha=0.95),
            arrowprops=dict(arrowstyle='->', color=COLOR_UP, lw=1.2)
        )
    else:
        ax_traj.text(
            0.5, 0.5,
            "No excursion episodes detected in empirical audit\n(run 'symm detect' to identify ZigZag/Friction episodes)",
            transform=ax_traj.transAxes, ha='center', va='center',
            fontsize=9.0, fontfamily='monospace', color=COLOR_WARN,
            bbox=dict(boxstyle='round,pad=0.4', facecolor=COLOR_RAISED, edgecolor=COLOR_WARN, alpha=0.9)
        )

    # Right Bottom: Sequitur Mechanism & State Over Time
    ax_seq_cr = ax_seq.twinx()
    ax_seq.set_xlim(0, total_steps - 1)
    ax_seq.set_ylim(-0.5, max(rules_hist) + 2.5)
    ax_seq_cr.set_ylim(0.95, max(comp_hist) * 1.15)

    seq_rule_line, = ax_seq.plot([], [], color=COLOR_WARN, linewidth=2.0, drawstyle='steps-post', label="Rules |R(t)|", zorder=3)
    seq_comp_line, = ax_seq_cr.plot([], [], color=COLOR_UP, linewidth=1.8, linestyle='-', label="Compression Ratio", zorder=2)
    cursor_seq = ax_seq.axvline(0, color=COLOR_DOWN, linestyle=':', linewidth=1.2, alpha=0.8)

    ax_seq.set_ylabel("Induced Rules |R(t)|", fontsize=9.0, fontfamily='monospace', color=COLOR_WARN)
    ax_seq.tick_params(axis='y', colors=COLOR_WARN)
    ax_seq_cr.set_ylabel("Grammar Compression (x)", fontsize=9.0, fontfamily='monospace', color=COLOR_UP)
    ax_seq_cr.tick_params(axis='y', colors=COLOR_UP)
    ax_seq.set_xlabel("Tape Step Index", fontsize=9.0, fontfamily='monospace', color=COLOR_F3)
    ax_seq.set_title("Sequitur Grammar Induction & Compression (Nevill-Manning 1997)", fontsize=10.5, fontfamily='monospace', color=COLOR_F1)
    ax_seq.grid(True, linestyle='--', alpha=0.25)

    # Rule event markers on Sequitur curve
    rule_dots, = ax_seq.plot([], [], marker='D', markersize=6, linestyle='None', color=COLOR_WARN, markeredgecolor=COLOR_F1, zorder=5)

    # Sequitur live HUD readout box
    seq_hud_text = ax_seq.text(
        0.03, 0.90, "",
        transform=ax_seq.transAxes,
        fontsize=8.0, fontfamily='monospace', color=COLOR_F1,
        bbox=dict(boxstyle='round,pad=0.25', facecolor=COLOR_RAISED, edgecolor=COLOR_LINE2, alpha=0.9),
        va='top', zorder=10
    )

    # Docked header static bar
    bar_rect = Rectangle((0.0, 0.865), 1.0, 0.135, facecolor=COLOR_SURFACE, edgecolor=None, transform=fig.transFigure, clip_on=False)
    fig.patches.append(bar_rect)
    div_line = plt.Line2D([0.0, 1.0], [0.865, 0.865], color=COLOR_LINE, linewidth=1.0, transform=fig.transFigure, clip_on=False)
    fig.lines.append(div_line)

    fig.text(0.025, 0.945, "SYMM Dynamic Excursion & Sequitur Sequence Memory", fontsize=12, fontweight='bold', fontfamily='sans-serif', color=COLOR_F1, va='center')
    fig.text(0.025, 0.898, "A/B/C excursion trajectory, cognitive predictions, and Sequitur grammar induction.", fontsize=8.5, fontfamily='monospace', color=COLOR_F3, va='center')

    # Status Badge: Derived from empirical audit stages
    dyn_pass = dynamics_data.get('passed', False)
    prec_pass = precursor_data.get('passed', False)
    cog_pass = cognitive_data.get('passed', False)

    if dyn_pass and prec_pass and cog_pass:
        badge_status = "● PASS"
        badge_tone = COLOR_UP
    elif not episodes or not phases:
        badge_status = "▲ INCOMPLETE"
        badge_tone = COLOR_WARN
    else:
        badge_status = "✖ FAIL"
        badge_tone = COLOR_DOWN

    badge_patch = FancyBboxPatch((0.85, 0.925), 0.12, 0.045, boxstyle="round,pad=0.006,rounding_size=0.008",
                                 facecolor=COLOR_RAISED, edgecolor=badge_tone, linewidth=1.0, transform=fig.transFigure, clip_on=False)
    fig.patches.append(badge_patch)
    fig.text(0.91, 0.947, badge_status, fontsize=8.5, fontweight='bold', fontfamily='monospace', color=badge_tone, ha='center', va='center')

    readout_text = fig.text(0.97, 0.895, "", fontsize=8.2, fontfamily='monospace', color=COLOR_F2, ha='right', va='center')

    def update(frame_idx):
        active_idx, ru_idx, profile = step_profiles[frame_idx]
        active_tok = active_tokens[frame_idx]
        cur_ret = cum_returns[frame_idx]
        cur_price = prices[frame_idx]
        cur_rules = rules_hist[frame_idx]
        cur_cr = comp_hist[frame_idx]

        if ep:
            phase_str = "PRECURSOR [A]" if frame_idx < idx_B else ("IGNITION [B]" if frame_idx < idx_C else "APEX REVERSAL [C]")
            if annot_A is not None:
                annot_A.set_alpha(1.0 if frame_idx >= idx_A else 0.35)
            if annot_B is not None:
                annot_B.set_alpha(1.0 if frame_idx >= idx_B else 0.30)
            if annot_C is not None:
                annot_C.set_alpha(1.0 if frame_idx >= idx_C else 0.25)
        else:
            phase_str = "TAPE STREAM"

        # 1. Update bars
        for r_i, bar in enumerate(bars):
            val = profile[r_i]
            bar.set_width(val)
            txt = bar_labels[r_i]
            txt.set_x(val + 0.02)
            txt.set_text(f"{val:.3f}")

            if r_i == active_idx:
                bar.set_color(COLOR_UP)
                bar.set_edgecolor(COLOR_F1)
                bar.set_linewidth(1.8)
                txt.set_color(COLOR_UP)
                txt.set_fontweight('bold')
            elif r_i == ru_idx and val > 0:
                bar.set_color(COLOR_WARN)
                bar.set_edgecolor(COLOR_WARN)
                bar.set_linewidth(1.2)
                txt.set_color(COLOR_WARN)
                txt.set_fontweight('bold')
            elif val > 0.25:
                bar.set_color(COLOR_INFO)
                bar.set_edgecolor(COLOR_LINE2)
                bar.set_linewidth(0.8)
                txt.set_color(COLOR_F2)
                txt.set_fontweight('normal')
            else:
                bar.set_color(COLOR_RAISED)
                bar.set_edgecolor(COLOR_LINE)
                bar.set_linewidth(0.6)
                txt.set_color(COLOR_F4)
                txt.set_fontweight('normal')

        # 2. Update Trajectory
        cur_x = np.arange(frame_idx + 1)
        trail_line.set_data(cur_x, cum_returns[:frame_idx + 1])
        head_dot.set_data([cur_x[-1]], [cum_returns[frame_idx]])
        cursor_traj.set_xdata([frame_idx, frame_idx])

        # 3. Update Sequitur
        seq_rule_line.set_data(cur_x, rules_hist[:frame_idx + 1])
        seq_comp_line.set_data(cur_x, comp_hist[:frame_idx + 1])
        cursor_seq.set_xdata([frame_idx, frame_idx])

        # Active rule dots
        past_events = [ev for ev in rule_events if ev[0] <= frame_idx]
        if past_events:
            ev_x = [ev[0] for ev in past_events]
            ev_y = [rules_hist[ev[0]] for ev in past_events]
            rule_dots.set_data(ev_x, ev_y)
        else:
            rule_dots.set_data([], [])

        r_info = active_rules_str[frame_idx]
        if not r_info:
            r_info = "Streaming Terminal Token"
        seq_hud_text.set_text(f"TOKEN: {active_tok}  |  {r_info}  |  RULES: {cur_rules}  |  COMPRESSION: {cur_cr:.2f}x")

        # 4. Update Header Readout
        price_str = f"${cur_price:,.1f}" if cur_price > 0 else ""
        if phases:
            cur_phase = phases[min(frame_idx, len(phases) - 1)]
            cur_act = cur_phase.get('action', 'WAIT').upper()
            cur_conf = cur_phase.get('confidence', 0.0)
            pred_str = f"PREDICT: {cur_act} ({cur_conf:.2f})"
        else:
            pred_str = "PREDICT: NONE"
        readout_text.set_text(
            f"STEP {frame_idx+1:02d}/{total_steps}  |  {active_tok}  |  {price_str}  |  {phase_str}  |  {pred_str}  |  {cur_rules} RULES ({cur_cr:.2f}x)"
        )

        return list(bars) + bar_labels + [trail_line, head_dot, cursor_traj, seq_rule_line, seq_comp_line, cursor_seq, rule_dots, seq_hud_text, readout_text]

    animation_obj = anim.FuncAnimation(fig, update, frames=total_steps, interval=250, blit=False)

    # 1. Save MP4 via ffmpeg
    if shutil.which('ffmpeg'):
        try:
            mp4_path = os.path.join(out_dir, 'excursion_transitions.mp4')
            animation_obj.save(mp4_path, writer='ffmpeg', fps=4, dpi=160)
            print(f"🎬 Generated excursion_transitions.mp4 at {mp4_path}")
        except Exception as e:
            print(f"⚠️ Warning: ffmpeg export failed: {e}", file=sys.stderr)

    # 2. Save GIF via Pillow
    try:
        gif_path = os.path.join(out_dir, 'excursion_transitions.gif')
        animation_obj.save(gif_path, writer='pillow', fps=4)
        print(f"🎞️ Generated excursion_transitions.gif at {gif_path}")
    except Exception as e:
        print(f"⚠️ Warning: gif export failed: {e}", file=sys.stderr)

    # 3. Save static companion milestone summary sheet with scanlines
    summary_path = os.path.join(out_dir, 'excursion_transitions_summary.png')
    milestone_idx = min(total_steps - 1, max(idx_C + 5, int(total_steps * 0.45))) if ep else int(total_steps * 0.5)
    update(milestone_idx)
    # Ensure full opacity for all annotations in summary sheet
    if ep:
        if annot_A is not None:
            annot_A.set_alpha(1.0)
        if annot_B is not None:
            annot_B.set_alpha(1.0)
        if annot_C is not None:
            annot_C.set_alpha(1.0)
    fig.savefig(summary_path, dpi=180, facecolor=COLOR_BG)
    apply_crt_scanlines(summary_path)
    plt.close(fig)
    print(f"🖼️ Generated excursion_transitions_summary.png at {summary_path}")


def main():
    parser = argparse.ArgumentParser(description="Render SYMM Audit Charts")
    parser.add_argument('positional_input', nargs='?', help="Path to audit_results.json")
    parser.add_argument('positional_output', nargs='?', help="Output directory")
    parser.add_argument('--input', dest='input_path', help="Path to audit_results.json")
    parser.add_argument('--output', dest='output_path', help="Output directory")
    args = parser.parse_args()

    json_path = args.input_path or args.positional_input or "audit_results/audit_results.json"
    out_dir = args.output_path or args.positional_output or "audit_results"

    plots_dir = out_dir if out_dir.endswith('plots') else os.path.join(out_dir, 'plots')
    os.makedirs(plots_dir, exist_ok=True)

    if not os.path.exists(json_path):
        print(f"❌ Error: {json_path} does not exist", file=sys.stderr)
        sys.exit(1)

    with open(json_path, 'r') as f:
        data = json.load(f)

    plot_stage0_contract(data, plots_dir)
    plot_stage1_timing(data, plots_dir)
    plot_stage2_vitality(data, plots_dir)
    plot_stage3_redundancy(data, plots_dir)
    plot_stage4_sympathy(data, plots_dir)
    plot_stage5_stability(data, plots_dir)
    plot_stage6_dynamics(data, plots_dir)
    plot_stage7_precursor(data, plots_dir)
    plot_stage8_cognitive(data, plots_dir)
    plot_stage9_truthfulness(data, plots_dir)
    plot_summary_dashboard(data, plots_dir)
    animate_excursion_transitions(data, plots_dir)

    print(f"✅ Generated 11 terminal-styled CRT raster diagnostic charts + dynamic transition animation in {plots_dir}")


if __name__ == '__main__':
    main()
