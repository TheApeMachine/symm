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
        boxstyle='round,pad=0.002,rounding_size=0.005',
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

        status = "PASS" if passed else "WARN"
        readout = f"{len(drifts)} STREAMS TRACKED  |  {breach_count} INVERSIONS"

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
    total_metrics = stage.get('total_metrics', 0)
    dead_count = stage.get('dead_count', 0)
    stagnant_count = stage.get('stagnant_count', 0)
    healthy_count = stage.get('healthy_count', max(0, total_metrics - dead_count - stagnant_count))

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))
    fig.subplots_adjust(top=0.80, bottom=0.12, left=0.08, right=0.96, wspace=0.28)

    categories = ['Healthy\n(Active)', 'Stagnant\n(>=80% Zero)', 'Dead\n(0 Variance)']
    counts = [healthy_count, stagnant_count, dead_count]
    colors = [COLOR_UP, COLOR_WARN, COLOR_DOWN]

    bars = ax1.bar(categories, counts, color=colors, width=0.52, edgecolor=COLOR_LINE)
    ax1.set_ylabel("Metric Count", fontsize=9.5, fontfamily='monospace', color=COLOR_F3)
    ax1.grid(axis='y')

    for bar in bars:
        h = bar.get_height()
        pct = (h / max(1, total_metrics)) * 100
        ax1.text(bar.get_x() + bar.get_width() / 2, h + max(1, total_metrics * 0.02),
                 f"{int(h)}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=8.5, fontfamily='monospace', color=COLOR_F1)

    metrics_map = stage.get('metrics', {})
    if metrics_map:
        top_by_var = sorted(metrics_map.items(), key=lambda kv: kv[1].get('variance', 0), reverse=True)[:10]
        names = [kv[0][:22] for kv in top_by_var]
        vars_vals = [kv[1].get('variance', 0) for kv in top_by_var]

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

    status = "HEALTHY" if dead_count == 0 else "ATTENTION"
    readout = f"{healthy_count}/{total_metrics} ACTIVE  |  {dead_count} DEAD  |  {stagnant_count} STAGNANT"

    render_docked_header(
        fig,
        "Stage 2: Sensor Vitality & Variance Health",
        "Identifies dead (zero variance) sensors and stagnant cold-starts across streaming observations.",
        status=status,
        readout=readout
    )
    save_plot(fig, os.path.join(out_dir, "stage2_metric_vitality.png"))


def plot_stage3_redundancy(data, out_dir):
    stage = data.get('redundancy', {})
    total_pairs = stage.get('total_pairs', 0)
    redundant_pairs = stage.get('redundant_pairs', [])
    redundant_count = stage.get('redundant_count', len(redundant_pairs))
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
        corrs = [abs(p.get('correlation', 0.95)) for p in top_pairs]

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
        readout = f"{len(estimates)} PAIRS EVALUATED  |  {redundant_count} EXTREME"

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
    passed = stage.get('passed', True)

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
    passed = stage.get('passed', True)

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
    taker_fee = stage.get('taker_fee', 0.008)
    excursions = stage.get('excursions', [])
    passed = stage.get('passed', True)

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
    passed = stage.get('passed', True)

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
        ("Stage 0 : Contract", data.get('contract', {}).get('passed', True)),
        ("Stage 1 : Timing", data.get('timing', {}).get('passed', True)),
        ("Stage 2 : Vitality", data.get('vitality', {}).get('passed', True)),
        ("Stage 3 : Redundancy", data.get('redundancy', {}).get('passed', True)),
        ("Stage 4 : Sympathy", data.get('sympathy', {}).get('passed', True)),
        ("Stage 5 : Stability", data.get('stability', {}).get('passed', True)),
        ("Stage 6 : Dynamics", data.get('dynamics', {}).get('passed', True)),
        ("Stage 7 : Precursor", data.get('precursor', {}).get('passed', True)),
        ("Stage 8 : Cognitive", data.get('cognitive', {}).get('passed', True)),
        ("Stage 9 : Truthfulness", data.get('truthfulness', {}).get('passed', True)),
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

    print(f"✅ Generated 11 terminal-styled CRT raster diagnostic charts in {plots_dir}")


if __name__ == '__main__':
    main()
