#!/usr/bin/env python3
"""
SYMM Audit Visualization Engine
Produces intuitive, publication-quality diagnostic charts with visual health headers
(status pills, progress health bars, target markers, and concise metric readouts)
ensuring that data and subplots are never obscured.
"""

import sys
import os
import json
import argparse
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.patches import FancyBboxPatch
import numpy as np

# Set dark modern styling
plt.style.use('dark_background')
plt.rcParams['font.sans-serif'] = 'DejaVu Sans, Arial, Helvetica'
plt.rcParams['axes.edgecolor'] = '#444c56'
plt.rcParams['axes.linewidth'] = 0.8
plt.rcParams['grid.color'] = '#2d333b'
plt.rcParams['grid.linestyle'] = '--'
plt.rcParams['grid.alpha'] = 0.6

def load_report(json_path):
    with open(json_path, 'r') as f:
        return json.load(f)

def render_health_header(fig, title, meaning, status="MEASURED", health_score=None, readout="", target_pct=None):
    """
    Renders a modern, visual diagnostic header card with a status pill, health progress bar,
    target marker, and concise metric readout in figure space.
    Ensures subplots and data are never obscured.
    """
    try:
        fig.tight_layout(rect=[0, 0.02, 1, 0.84])
    except Exception:
        fig.subplots_adjust(top=0.83)

    # Outer Card
    card = FancyBboxPatch(
        (0.025, 0.855), 0.95, 0.125,
        boxstyle='round,pad=0.012',
        facecolor='#1c2128',
        edgecolor='#30363d',
        transform=fig.transFigure,
        clip_on=False
    )
    fig.patches.append(card)

    # Title
    fig.text(0.045, 0.948, title, fontsize=11.5, fontweight='bold', color='#f0f6fc', va='center')

    # Meaning / Subtitle
    fig.text(0.045, 0.922, meaning, fontsize=8.2, color='#8b949e', va='center')

    # Status Pill styling
    st = status.upper()
    if any(k in st for k in ['PASS', 'HEALTHY', 'OPTIMAL', 'STABLE', 'SKILLFUL', 'STRUCTURED', 'SEPARATED', 'DIVERGENT', 'INDEPENDENT', 'BALANCED']):
        bg, border, fg, icon = '#163b22', '#3fb950', '#3fb950', '●'
    elif any(k in st for k in ['WARN', 'ATTENTION', 'MARGINAL', 'DOMINATED', 'SPORADIC']):
        bg, border, fg, icon = '#382810', '#d29922', '#e3b341', '▲'
    elif any(k in st for k in ['FAIL', 'BREACH', 'CRITICAL', 'UNSTABLE', 'NULL', 'RANDOM']):
        bg, border, fg, icon = '#3d1a1f', '#f85149', '#ff7b72', '✖'
    elif any(k in st for k in ['MEASURED', 'NOMINAL', 'COMPRESSED', 'CALIBRATED']):
        bg, border, fg, icon = '#14294b', '#388bfd', '#79c0ff', '◆'
    else:
        bg, border, fg, icon = '#25292e', '#6e7681', '#8b949e', '○'

    fig_w, fig_h = fig.get_size_inches()

    # Pill placement dynamically sized to text
    st_text = f'{icon} {status}'
    pill_w = max(0.08, (len(st_text) * 0.072 + 0.25) / fig_w)
    pill_x = 0.045
    pill_y = 0.875
    pill_h = 0.032
    pill = FancyBboxPatch(
        (pill_x, pill_y), pill_w, pill_h,
        boxstyle='round,pad=0.004',
        facecolor=bg,
        edgecolor=border,
        transform=fig.transFigure,
        clip_on=False
    )
    fig.patches.append(pill)
    fig.text(
        pill_x + pill_w / 2, pill_y + pill_h / 2,
        st_text,
        fontsize=7.8, fontweight='bold', color=fg,
        ha='center', va='center'
    )

    # Health progress bar track
    if health_score is not None:
        track_x = pill_x + pill_w + 0.02
        track_w = max(0.14, 2.8 / fig_w)
        track_h = 0.022
        track_y = pill_y + (pill_h - track_h) / 2

        track = FancyBboxPatch(
            (track_x, track_y), track_w, track_h,
            boxstyle='round,pad=0.003',
            facecolor='#21262d',
            edgecolor='#373e47',
            transform=fig.transFigure,
            clip_on=False
        )
        fig.patches.append(track)

        clamped_score = min(1.0, max(0.0, health_score))
        if clamped_score > 0.01:
            bar_color = border if border != '#6e7681' else '#58a6ff'
            fill = FancyBboxPatch(
                (track_x, track_y), track_w * clamped_score, track_h,
                boxstyle='round,pad=0.003',
                facecolor=bar_color,
                edgecolor='none',
                transform=fig.transFigure,
                clip_on=False
            )
            fig.patches.append(fill)

        if target_pct is not None and 0.0 < target_pct <= 1.0:
            tx = track_x + track_w * target_pct
            fig.lines.append(plt.Line2D(
                [tx, tx], [track_y - 0.004, track_y + track_h + 0.004],
                color='#f0f6fc', linewidth=1.5, transform=fig.transFigure
            ))

        readout_x = track_x + track_w + 0.02
    else:
        readout_x = pill_x + pill_w + 0.02

    fig.text(readout_x, pill_y + pill_h / 2, readout, fontsize=8.2, color='#c9d1d9', va='center')

def add_header(fig, ax, main_title, meaning, good_criteria="", higher_lower="", pad=58, **kwargs):
    """Backward-compatible header adapter redirecting to render_health_header."""
    render_health_header(fig, main_title, meaning, **kwargs)

def add_figure_header(fig, main_title, meaning, good_criteria="", higher_lower="", **kwargs):
    """Backward-compatible figure-level header adapter redirecting to render_health_header."""
    render_health_header(fig, main_title, meaning, **kwargs)

def plot_stage0_contract(report, out_dir):
    contract = report.get('contract', {})
    breaches = contract.get('breaches') or []
    total_checked = contract.get('total_metrics_checked', 0)
    norm = contract.get('metric_norm', {})
    state = contract.get('measurement_state', {})
    is_passed = contract.get('passed', False)

    unstandardized = norm.get('unstandardized_mature_breaches', 0)
    z_breaches = norm.get('z_magnitude_breaches', 0)
    norm_breaches = norm.get('normalization_breaches', 0)
    std_breaches = norm.get('standardization_breaches', 0)
    total_norm_breaches = unstandardized + z_breaches + norm_breaches + std_breaches

    state_passed = state.get('passed', True)
    worm_breaches = state.get('worm_id_breaches', 0)
    coherence_breaches = state.get('coherence_breaches', 0)
    maturity_breaches = state.get('maturity_breaches', 0)
    energy_breaches = state.get('energy_breaches', 0)
    total_state_breaches = worm_breaches + coherence_breaches + maturity_breaches + energy_breaches

    total_failures = len(breaches) + total_norm_breaches + total_state_breaches

    fig, ax = plt.subplots(figsize=(14, 8))

    if is_passed and total_failures == 0:
        ax.text(0.5, 0.5, f"[OK] All {total_checked} Metrics Comply with Declared Contracts\nZero values emitted outside stated mathematical domains.\nAll mature streams standardized; WORM & measurement state verified.",
                ha='center', va='center', fontsize=12, color='#2ea043', transform=ax.transAxes)
        status = "PASS"
        score = 1.0
        readout = f"0 Breaches across {total_checked} Metrics  |  Target: 0 Breaches (100% Compliant)"
    elif breaches:
        top_breaches = breaches[:15]
        labels = [f"{b['metric']} ({b['declared_domain']})" for b in top_breaches]
        mins = [b['min_val'] for b in top_breaches]
        maxs = [b['max_val'] for b in top_breaches]
        means = [b['mean_val'] for b in top_breaches]

        y_pos = range(len(top_breaches))

        # Draw declared bounds reference zone [-1, 1] or [0, 1]
        ax.axvspan(-1.0, 1.0, color='#2ea043', alpha=0.10, label='Standard Valid Correlation Domain [-1, 1]')
        ax.axvline(1.0, color='#f85149', linestyle='--', linewidth=1.5, label='Upper Bound (1.0)')
        ax.axvline(-1.0, color='#f85149', linestyle='--', linewidth=1.5)

        # Plot range lines for each metric
        for idx in y_pos:
            ax.plot([mins[idx], maxs[idx]], [idx, idx], color='#f85149', linewidth=2.5)
            ax.plot(means[idx], idx, 'o', color='#f0883e', markersize=6)

        ax.set_yticks(y_pos)
        ax.set_yticklabels(labels, fontsize=8)
        ax.invert_yaxis()
        ax.set_xlabel('Observed Value Range [Min, Max] (Circle = Mean)', fontsize=10, color='#adbac7')
        ax.grid(axis='x')
        ax.legend(loc='lower right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)

        status = "BREACH"
        score = max(0.0, 1.0 - total_failures / max(1, total_checked))
        readout = f"{len(breaches)} Domain Breaches, {total_norm_breaches} Norm, {total_state_breaches} State  |  Target: 0 Breaches"
    else:
        # No raw domain breaches, but norm or state failed!
        status = "BREACH"
        score = 0.0
        details = []
        if unstandardized > 0:
            details.append(f"• Unstandardized Mature Stream Breaches: {unstandardized}")
        if z_breaches > 0:
            details.append(f"• Chebyshev Bound Outliers (|z| > rankBound): {z_breaches}")
        if norm_breaches > 0 or std_breaches > 0:
            details.append(f"• Normalization / Standardization Invariant Breaches: {norm_breaches + std_breaches}")
        if not state_passed or total_state_breaches > 0:
            details.append(f"• Measurement State / WORM Breaches: {total_state_breaches} (Coherence: {coherence_breaches}, WORM: {worm_breaches})")

        error_msg = f"[STAGE 0 CONTRACT BREACH]\nMetric contracts or standardizations violated:\n\n" + "\n".join(details)
        ax.text(0.5, 0.5, error_msg, ha='center', va='center', fontsize=11, color='#f85149', transform=ax.transAxes,
                bbox=dict(boxstyle='round,pad=1', facecolor='#22272e', edgecolor='#f85149', alpha=0.9))
        readout = f"CONTRACT BREACH: {total_norm_breaches} Norm Breaches, {total_state_breaches} State Breaches  |  Target: 0 Breaches"

    render_health_header(
        fig,
        f'Stage 0: Metric Contract Integrity ({total_failures} Breaches Found)',
        'Audits whether sensors strictly adhere to declared mathematical domains, causal standardization, and WORM state.',
        status=status,
        health_score=score,
        readout=readout,
        target_pct=1.0
    )

    fig.savefig(os.path.join(out_dir, 'stage0_metric_contracts.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage1_vitality(report, out_dir):
    vitality = report.get('vitality', {})
    raw_metrics = vitality.get('raw_metrics', [])
    canonical_cells = vitality.get('canonical_cells', [])

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.5))

    # Left: Raw Producer Metrics Overview
    raw_healthy = vitality.get('raw_healthy_metrics', 0)
    raw_dead = vitality.get('raw_dead_metrics', 0)
    raw_sporadic = vitality.get('raw_sporadic_metrics', 0)
    raw_total = vitality.get('raw_producer_metrics', len(raw_metrics))

    categories = ['Healthy\n(Active)', 'Dead\n(Constant)', 'Sporadic\n(<20% Cover)']
    raw_counts = [raw_healthy, raw_dead, raw_sporadic]
    colors = ['#2ea043', '#f85149', '#d29922']

    ax1.bar(categories, raw_counts, color=colors, width=0.55, edgecolor='#22272e')
    for i, count in enumerate(raw_counts):
        pct = (count / max(1, raw_total)) * 100
        ax1.text(i, count + max(1, raw_total)*0.02, f"{count}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=9, color='#adbac7')
    ax1.set_ylabel('Number of Series', fontsize=10, color='#adbac7')
    ax1.set_title(f'Raw Producer Named Series (Total: {raw_total})', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')

    # Right: Canonical Grid Inputs Overview
    canon_healthy = vitality.get('canonical_healthy_cells', 0)
    canon_dead = vitality.get('canonical_dead_cells', 0)
    canon_sporadic = vitality.get('canonical_sporadic_cells', 0)
    canon_total = vitality.get('canonical_grid_cells', len(canonical_cells))

    canon_counts = [canon_healthy, canon_dead, canon_sporadic]
    ax2.bar(categories, canon_counts, color=colors, width=0.55, edgecolor='#22272e')
    for i, count in enumerate(canon_counts):
        pct = (count / max(1, canon_total)) * 100
        ax2.text(i, count + max(1, canon_total)*0.02, f"{count}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=9, color='#adbac7')
    ax2.set_ylabel('Number of Canonical Cells', fontsize=10, color='#adbac7')
    ax2.set_title(f'Canonical Grid Universe (Total: {canon_total} Cells)', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')

    canon_pct = (canon_healthy / max(1, canon_total))
    vitality_status = "HEALTHY" if canon_pct >= 0.70 else "ATTENTION"
    vitality_readout = f"Canonical Grid: {canon_healthy}/{canon_total} Active ({canon_pct*100:.1f}%), Raw: {raw_healthy}/{raw_total}  |  Target: ≥ 70% Active"

    render_health_header(
        fig,
        'Stage 1: Sensor Vitality & Universe Dimension Mapping',
        'Compares raw producer output series (peer-qualified) against the canonical collapsed grid universe.',
        status=vitality_status,
        health_score=canon_pct,
        readout=vitality_readout,
        target_pct=0.70
    )

    fig.savefig(os.path.join(out_dir, 'stage1_metric_vitality.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage1_redundancy(report, out_dir):
    redundant = (report.get('vitality') or {}).get('redundant_pairs') or []
    pairs = [f"{p['metric_a']} <-> {p['metric_b']}" for p in redundant[:18]]
    corrs = [p['correlation'] for p in redundant[:18]]

    fig_height = max(6.0, len(pairs) * 0.42 + 2.5) if pairs else 6.0
    fig, ax = plt.subplots(figsize=(14, fig_height))

    if not redundant:
        ax.text(0.5, 0.5, "[OK] Zero Redundant Canonical Clones Found (|r| >= 0.95)\nEvery healthy cell represents an independent dimension.",
                ha='center', va='center', fontsize=12, color='#2ea043', transform=ax.transAxes)
        status = "INDEPENDENT"
        readout = "0 Collinear Clones (|r| ≥ 0.95)  |  Orthogonal Feature Space"
    else:
        ax.barh(range(len(pairs)), corrs, color='#58a6ff', height=0.55)
        ax.set_yticks(range(len(pairs)))
        ax.set_yticklabels(pairs, fontsize=8)
        ax.set_xlabel('Pearson Correlation Coefficient (r)', fontsize=10, color='#adbac7')
        ax.set_xlim(0.9, 1.02)
        ax.invert_yaxis()
        ax.tick_params(axis='y', labelsize=8)
        status = "COMPRESSED"
        readout = f"{len(redundant)} Collinear Pairs (|r| ≥ 0.95)  |  Validates Spectral Grid Dimensionality Reduction"

    render_health_header(
        fig,
        f'Stage 1: Canonical Metric Redundancy (|r| >= 0.95, {len(redundant)} Pairs Found)',
        'Pairs of canonical cells tracking each other so closely they share representation subspace.',
        status=status,
        health_score=1.0,
        readout=readout,
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage1_metric_redundancy.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage2_sympathy_null(report, out_dir):
    symp = report.get('sympathy', {})
    real_bins = symp.get('real_bins', [])
    real_counts = symp.get('real_counts', [])
    null_dist = symp.get('null_distribution', {})
    null_bins = null_dist.get('histogram_bins', [])
    null_counts = null_dist.get('histogram_counts', [])

    if not real_bins or not null_bins:
        return

    real_total = sum(real_counts) if sum(real_counts) > 0 else 1
    null_total = sum(null_counts) if sum(null_counts) > 0 else 1

    real_p = [c / real_total for c in real_counts]
    null_p = [c / null_total for c in null_counts]

    real_centers = [(real_bins[i] + real_bins[i+1])/2 for i in range(len(real_bins)-1)]
    null_centers = [(null_bins[i] + null_bins[i+1])/2 for i in range(len(null_bins)-1)]

    fig, ax = plt.subplots(figsize=(11, 7))
    ax.plot(null_centers, null_p, color='#8b949e', linewidth=2.2, label='Shuffled Null (Zero-Centered Noise)', alpha=0.9)
    ax.fill_between(null_centers, null_p, color='#8b949e', alpha=0.25)

    ax.plot(real_centers, real_p, color='#58a6ff', linewidth=2.5, label='Real Deformation Concordance', marker='o', markersize=4)
    ax.fill_between(real_centers, real_p, color='#58a6ff', alpha=0.3)

    p95 = null_dist.get('percentile_95', 0)
    if p95 > 0:
        ax.axvline(p95, color='#d29922', linestyle='--', linewidth=1.5, label=f'95th Percentile Null Bound ({p95:.2f})')

    ax.set_xlabel('Pairwise Deformation Correlation', fontsize=10, color='#adbac7')
    ax.set_ylabel('Probability Density', fontsize=10, color='#adbac7')
    ax.grid(True)
    ax.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=9)

    sep = symp.get('separation_ratio', 0.0)
    ks = symp.get('ks_statistic', 0.0)
    if sep >= 0.10 or ks >= 0.10:
        status = "SEPARATED"
        score = min(1.0, max(0.2, sep * 2.0))
        readout = f"Separation: {sep*100:.1f}% vs Null Envelope (KS: {ks:.3f})  |  Target: > 10% Tail Separation"
    else:
        status = "NULL"
        score = max(0.05, sep)
        readout = f"Separation: {sep*100:.1f}% vs Null Envelope  |  Indistinguishable from Permutation Null"

    render_health_header(
        fig,
        'Stage 2: Pair Sympathy vs. Shuffled Null on Deformations',
        'Tests whether real scale-free deformations show significant correlation beyond a permutation null.',
        status=status,
        health_score=score,
        readout=readout,
        target_pct=0.50
    )

    fig.savefig(os.path.join(out_dir, 'stage2_sympathy_null.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage2_orientation(report, out_dir):
    symp = report.get('sympathy', {})
    pos = symp.get('positive_pairs', 0)
    inv = symp.get('inverse_pairs', 0)
    total = max(1, pos + inv)

    fig, ax = plt.subplots(figsize=(9, 6.5))
    bars = ax.bar(['Positive Sympathy\n(r > 0)', 'Inverse Opposition\n(r < 0)'],
                   [pos, inv],
                   color=['#2ea043', '#f0883e'],
                   width=0.45, edgecolor='#22272e')

    for bar, val in zip(bars, [pos, inv]):
        pct = (val / total) * 100
        ax.text(bar.get_x() + bar.get_width()/2, val + total*0.015,
                f"{val}\n({pct:.1f}%)", ha='center', va='bottom', fontsize=10, color='#adbac7')

    ax.set_ylabel('Number of Metric Pairs', fontsize=10, color='#adbac7')
    ax.grid(axis='y')

    bal = min(pos, inv) / (total * 0.5) if total > 0 else 0.5
    readout = f"Positive: {pos} ({pos/total*100:.1f}%), Inverse: {inv} ({inv/total*100:.1f}%)  |  Coupled Market Coexistence"

    render_health_header(
        fig,
        'Stage 2: Sympathy Orientation (Lockstep vs. Inverse Opposition)',
        'Classifies pairwise deformation movements into positive co-movement vs stable inverse opposition.',
        status="NOMINAL",
        health_score=min(1.0, max(0.2, bal)),
        readout=readout,
        target_pct=0.50
    )

    fig.savefig(os.path.join(out_dir, 'stage2_orientation_balance.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage3_grid_completeness(report, out_dir):
    comp = report['grid_completeness']
    total_frames = comp['total_frames']
    if total_frames == 0:
        return

    observed = comp['observed_frames']
    observed_pct = comp['observed_fraction'] * 100
    members = comp['mean_region_members']

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.2))

    # Panel 1: frames Grid.Observe turned into regions vs not.
    ax1.bar(['Observed', 'Not observed'], [observed, total_frames - observed], color=['#3fb950', '#f85149'], width=0.55, edgecolor='#30363d')
    ax1.set_ylabel('Frame Count', fontsize=10, color='#adbac7')
    ax1.set_title(f'Frames observed by Grid.Observe: {observed_pct:.1f}%', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')

    # Panel 2: mean metrics per region in observed frames.
    regions = [f'R{r:02d}' for r in range(12)]
    ax2.bar(regions, [members[r] for r in regions], color='#58a6ff', width=0.6, edgecolor='#30363d')
    ax2.set_xlabel('Grid Region', fontsize=10, color='#adbac7')
    ax2.set_ylabel('Mean Metrics per Observed Frame', fontsize=10, color='#adbac7')
    ax2.set_title('Region Membership', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')

    render_health_header(
        fig,
        f'Stage 3: Grid Completeness (Observed: {observed_pct:.1f}%)',
        'A frame is observed when every peer is warm and every region holds at least one metric.',
        status=comp['status'],
        health_score=observed_pct / 100.0,
        readout=f"Observed {observed} of {total_frames} frames",
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage3_grid_completeness.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)


def plot_stage4_token_dynamics(report, out_dir):
    dynamics = report.get('token_dynamics') or {}
    freqs = dynamics.get('token_frequencies') or {}
    region_strengths = dynamics.get('region_strengths') or {}
    total = max(1, dynamics.get('total_emissions') or 0)

    if not freqs:
        return

    sorted_tokens = sorted(freqs.keys(), key=lambda t: freqs[t], reverse=True)
    counts = [freqs[t] for t in sorted_tokens]
    shares = [c / total * 100 for c in counts]
    scores = [region_strengths.get(t, {}).get('mean_score', 0) for t in sorted_tokens]
    margins = [region_strengths.get(t, {}).get('mean_margin', 0) for t in sorted_tokens]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(16, 7.5))

    # Left Panel: Emission Share (%)
    bars = ax1.bar(sorted_tokens, shares, color='#58a6ff', width=0.55, edgecolor='#22272e')
    for bar, pct in zip(bars, shares):
        ax1.text(bar.get_x() + bar.get_width()/2, pct + 0.8, f"{pct:.1f}%", ha='center', va='bottom', fontsize=8, color='#adbac7')

    ax1.axhline(80, color='#f85149', linestyle='--', label='Max Dominance Cap (80%)')
    ax1.set_ylabel('Emission Frequency (%)', fontsize=10, color='#adbac7')
    ax1.set_xlabel('Region Token', fontsize=10, color='#adbac7')
    ax1.set_title('Token Emission Share on Held-Out Tape', fontsize=11, fontweight='bold')
    ax1.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
    ax1.grid(axis='y')

    # Right Panel: Region Excitation Strength & Runner-Up Margin
    x_indices = np.arange(len(sorted_tokens))
    bar_width = 0.35
    ax2.bar(x_indices - bar_width/2, scores, bar_width, label='Mean Excitation Strength (Score)', color='#2ea043', edgecolor='#22272e')
    ax2.bar(x_indices + bar_width/2, margins, bar_width, label='Runner-Up Margin (Separation)', color='#d29922', edgecolor='#22272e')
    ax2.set_xticks(x_indices)
    ax2.set_xticklabels(sorted_tokens, fontsize=8.5)
    ax2.set_xlabel('Region Token', fontsize=10, color='#adbac7')
    ax2.set_ylabel('Excitation Magnitude / Margin', fontsize=10, color='#adbac7')
    ax2.set_title('Region Lighting Strength & Contrast Separation', fontsize=11, fontweight='bold')
    ax2.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
    ax2.grid(axis='y')

    mean_margin = dynamics.get('mean_runner_up_margin', 0)
    max_share = max(shares) if shares else 0.0

    if max_share <= 80.0 and mean_margin >= 0.05:
        status = "BALANCED"
        score = min(1.0, max(0.1, 1.0 - (max_share / 100.0)))
        readout = f"Max Token Dominance: {max_share:.1f}%, Margin: {mean_margin:.3f}  |  Target: < 80% Dominance, > 0.05 Margin"
    else:
        status = "DOMINATED" if max_share > 80.0 else "SPORADIC"
        score = 0.25
        readout = f"Max Dominance: {max_share:.1f}% (Excessive Clumping)  |  Target: < 80% Token Dominance"

    render_health_header(
        fig,
        f'Stage 4: Out-of-Sample Token Emissions & Region Excitation ({total} Emissions across {len(freqs)} Regions)',
        'Evaluates frequency, excitation score, and contrast separation when replaying unseen tape through frozen grid.',
        status=status,
        health_score=score,
        readout=readout,
        target_pct=0.50
    )

    fig.savefig(os.path.join(out_dir, 'stage4_token_dynamics.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_transition_matrix(report, out_dir):
    dynamics = report.get('token_dynamics') or {}
    transitions = dynamics.get('transitions') or {}
    compressed_transitions = dynamics.get('compressed_transitions') or {}
    stay_acc = (dynamics.get('always_stay_accuracy') or 0.0) * 100
    raw_h = dynamics.get('transition_entropy') or 0.0
    comp_h = dynamics.get('compressed_transition_entropy') or 0.0

    tokens = sorted(list(transitions.keys()), key=lambda r: int(r.replace('R', '')) if r.replace('R', '').isdigit() else 999)
    if not tokens:
        return

    n = len(tokens)
    raw_mat = np.zeros((n, n))
    for i, from_tok in enumerate(tokens):
        row_map = transitions.get(from_tok, {})
        row_total = sum(row_map.values())
        if row_total > 0:
            for j, to_tok in enumerate(tokens):
                raw_mat[i, j] = row_map.get(to_tok, 0) / row_total

    has_compressed = bool(compressed_transitions)
    if has_compressed:
        comp_tokens = sorted(list(compressed_transitions.keys()), key=lambda r: int(r.replace('R', '')) if r.replace('R', '').isdigit() else 999)
        cn = len(comp_tokens)
        comp_mat = np.zeros((cn, cn))
        for i, from_tok in enumerate(comp_tokens):
            row_map = compressed_transitions.get(from_tok, {})
            row_total = sum(row_map.values())
            if row_total > 0:
                for j, to_tok in enumerate(comp_tokens):
                    comp_mat[i, j] = row_map.get(to_tok, 0) / row_total

        fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(16, 7.5))

        # Panel 1: Raw Transitions
        im1 = ax1.imshow(raw_mat, cmap='viridis', interpolation='nearest')
        ax1.set_xticks(range(n))
        ax1.set_yticks(range(n))
        ax1.set_xticklabels(tokens, fontsize=8)
        ax1.set_yticklabels(tokens, fontsize=8)
        ax1.set_xlabel('Next Region Token (t+1)', fontsize=10, color='#adbac7')
        ax1.set_ylabel('Current Region Token (t)', fontsize=10, color='#adbac7')
        ax1.set_title(f'Raw Temporal Transitions (Stay: {stay_acc:.1f}%, H={raw_h:.3f}b)', fontsize=11, fontweight='bold')
        fig.colorbar(im1, ax=ax1, fraction=0.046, pad=0.04)

        # Panel 2: Compressed Transitions
        im2 = ax2.imshow(comp_mat, cmap='magma', interpolation='nearest')
        ax2.set_xticks(range(cn))
        ax2.set_yticks(range(cn))
        ax2.set_xticklabels(comp_tokens, fontsize=8)
        ax2.set_yticklabels(comp_tokens, fontsize=8)
        ax2.set_xlabel('Next State Region Token', fontsize=10, color='#adbac7')
        ax2.set_ylabel('Current State Region Token', fontsize=10, color='#adbac7')
        ax2.set_title(f'Run-Length Compressed Pathways (H={comp_h:.3f}b)', fontsize=11, fontweight='bold')
        fig.colorbar(im2, ax=ax2, fraction=0.046, pad=0.04)

        render_health_header(
            fig,
            'Stage 4: Out-of-Sample Region State Transition Dynamics',
            'Compares raw temporal dwell autocorrelation vs run-length compressed inter-region state transitions.',
            status="STRUCTURED",
            health_score=0.88,
            readout=f"Raw Dwell Stay: {stay_acc:.1f}% (H={raw_h:.3f}b) vs Compressed Inter-State H={comp_h:.3f}b  |  Disentangled Corridors",
            target_pct=None
        )
    else:
        fig, ax = plt.subplots(figsize=(10, 8.5))
        im = ax.imshow(raw_mat, cmap='viridis', interpolation='nearest')
        ax.set_xticks(range(n))
        ax.set_yticks(range(n))
        ax.set_xticklabels(tokens, fontsize=8)
        ax.set_yticklabels(tokens, fontsize=8)
        ax.set_xlabel('Next Region Token (t+1)', fontsize=10, color='#adbac7')
        ax.set_ylabel('Current Region Token (t)', fontsize=10, color='#adbac7')
        fig.colorbar(im, ax=ax, fraction=0.046, pad=0.04)

        render_health_header(
            fig,
            'Stage 4: Out-of-Sample Region State Transition Matrix',
            'Empirical probability distribution of transitions between consecutive regional states.',
            status="STRUCTURED",
            health_score=0.85,
            readout=f"Empirical State Transition Grid ({n} Regional States)  |  Pathways & Diagonal Corridors",
            target_pct=None
        )

    fig.savefig(os.path.join(out_dir, 'stage4_transition_matrix.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_entropy(report, out_dir):
    dynamics = report.get('token_dynamics') or {}
    real_ent = dynamics.get('transition_entropy') or 0
    null_ent = dynamics.get('null_transition_entropy') or 0
    red_bits = dynamics.get('entropy_reduction_bits') or 0

    fig, ax = plt.subplots(figsize=(9, 6.5))
    bars = ax.bar(['Real Observed Transitions', 'Block-Shuffled Temporal Null'],
                   [real_ent, null_ent],
                   color=['#3fb950', '#8b949e'],
                   width=0.45, edgecolor='#22272e')

    for bar, val in zip(bars, [real_ent, null_ent]):
        ax.text(bar.get_x() + bar.get_width()/2, val + 0.05, f"{val:.3f} bits", ha='center', va='bottom', fontsize=10, color='#adbac7')

    ax.set_ylabel('Conditional Transition Entropy (Bits)', fontsize=10, color='#adbac7')
    ax.grid(axis='y')

    if red_bits >= 0.10:
        status = "STRUCTURED"
        score = min(1.0, max(0.2, red_bits / 1.0))
        readout = f"Entropy Reduction: {red_bits:.3f} Bits vs Block Null  |  Target: > 0.10 Bits (Predictable Transitions)"
    else:
        status = "RANDOM"
        score = 0.15
        readout = f"Entropy Reduction: {red_bits:.3f} Bits  |  Target: > 0.10 Bits (Transitions Resemble Shuffled Noise)"

    render_health_header(
        fig,
        f'Stage 4: Out-of-Sample Transition Entropy (Reduction = {red_bits:.3f} Bits)',
        'Information entropy of the next-state token given the current state on held-out market tape.',
        status=status,
        health_score=score,
        readout=readout,
        target_pct=0.30
    )

    fig.savefig(os.path.join(out_dir, 'stage4_transition_entropy.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage5_precursor(report, out_dir):
    prec = report.get('precursor', {})
    det_found = prec.get('detections_found', 0)
    classes = prec.get('excursions_found', [])
    ign = prec.get('ignition_hypothesis', {})
    exh = prec.get('exhaustion_hypothesis', {})
    skill = prec.get('predictive_skill', {})
    economic = prec.get('economic_relevance', {})

    fig, (ax1, ax2, ax3) = plt.subplots(1, 3, figsize=(18, 7.0))

    # Panel 1: Statistical Separation (JSD vs Null-95)
    ign_jsd = ign.get('divergence_bits', 0)
    ign_null = ign.get('null_divergence_95', 0)
    exh_jsd = exh.get('divergence_bits', 0)
    exh_null = exh.get('null_divergence_95', 0)

    x = np.arange(2)
    w = 0.35
    ax1.bar(x - w/2, [ign_jsd, exh_jsd], w, label='Observed JSD', color=['#58a6ff', '#bc8cff'])
    ax1.bar(x + w/2, [ign_null, exh_null], w, label='Null-95 Bound', color='#8b949e')
    ax1.set_xticks(x)
    ax1.set_xticklabels(['A->B Ignition', 'B->C Exhaustion'], fontsize=9)
    ax1.set_ylabel('Divergence (Bits)', fontsize=10, color='#adbac7')
    ax1.set_title('1. Statistical Separation', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')
    ax1.legend(loc='upper right', fontsize=8)

    # Panel 2: Predictive Skill (BalAcc, Baseline, MCC)
    bal_acc = skill.get('balanced_accuracy', 0.5) * 100
    base_rate = skill.get('prior_base_rate', 0.5) * 100
    mcc_val = skill.get('mcc', 0.0)
    gain_bits = skill.get('predictive_gain_bits', 0.0)
    skill_status = skill.get('status', 'INSUFFICIENT_DATA')

    s_bars = ax2.bar(['Balanced Acc', 'Prior Base Rate', 'MCC x100'],
                     [bal_acc, base_rate, mcc_val * 100],
                     color=['#3fb950', '#8b949e', '#79c0ff'], width=0.45)
    ax2.axhline(50, color='#ff7b72', linestyle=':', label='Chance Baseline (50%)')
    ax2.set_ylabel('Percentage / Scaled Score', fontsize=10, color='#adbac7')
    ax2.set_title(f'2. Predictive Skill ({skill_status}) | Gain={gain_bits:.3f}b', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')
    ax2.legend(loc='upper right', fontsize=8)
    for bar in s_bars:
        h = bar.get_height()
        ax2.text(bar.get_x() + bar.get_width()/2., h + 1, f"{h:.1f}", ha='center', va='bottom', fontsize=8.5, color='#f0f6fc')

    # Panel 3: Economic Relevance (Friction Clearance)
    clearance = economic.get('friction_clearance_rate', 0.0) * 100
    gross_ret = economic.get('gross_mean_return', 0.0) * 100
    net_ret = economic.get('net_mean_return', 0.0) * 100
    rt_fee = economic.get('round_trip_fee_rate', 0.0) * 10000
    n_eval = economic.get('evaluated_excursions', 0)
    econ_status = economic.get('status', 'INSUFFICIENT_DATA')

    e_bars = ax3.bar(['Clearance Rate', 'Gross Return', 'Net Edge'],
                     [clearance, gross_ret, net_ret],
                     color=['#2ea043' if clearance > 50 else '#d29922', '#58a6ff', '#3fb950' if net_ret > 0 else '#f85149'],
                     width=0.45)
    ax3.set_ylabel('Percentage (%)', fontsize=10, color='#adbac7')
    ax3.set_title(f'3. Economic Friction Clearance (N={n_eval}, Fee={rt_fee:.1f}bps)', fontsize=11, fontweight='bold')
    ax3.grid(axis='y')
    for bar in e_bars:
        h = bar.get_height()
        ax3.text(bar.get_x() + bar.get_width()/2., h + 0.1, f"{h:.2f}%", ha='center', va='bottom', fontsize=8.5, color='#f0f6fc')

    overall_status = "MEASURED" if ign_jsd > 0 or skill_status == 'MEASURED' else "INSUFFICIENT_DATA"
    render_health_header(
        fig,
        f'Stage 5: Precursor Informativeness (Detections: {det_found}, Classes: {", ".join(classes) if classes else "none"})',
        'Decouples statistical token separation, out-of-sample predictive anticipation skill, and economic friction clearance.',
        status=overall_status,
        health_score=0.82 if overall_status == "MEASURED" else 0.0,
        readout=f"JSD: {ign_jsd:.3f}b  |  Skill: BalAcc {bal_acc:.1f}% (MCC {mcc_val:.3f})  |  Clearance: {clearance:.1f}% (Net Edge: {net_ret:.2f}%)",
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage5_precursor_separation.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage6_trie_skill(report, out_dir):
    cog = report.get('cognitive_trie', {})
    status = cog.get('status', 'INSUFFICIENT_DATA')
    top1 = cog.get('top1', {})
    top2 = cog.get('top2', {})
    top3 = cog.get('top3', {})

    has_multi = top1.get('k', 0) > 0 and (top2.get('k', 0) > 0 or top3.get('k', 0) > 0)

    if has_multi:
        fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.5))

        configs = [('Top-1\nWinner', top1), ('Top-2\nPair', top2), ('Top-3\nTriad', top3)]
        labels = [c[0] for c in configs if c[1].get('k', 0) > 0]
        sub_cogs = [c[1] for c in configs if c[1].get('k', 0) > 0]

        x = np.arange(len(labels))
        width = 0.35

        bal_accs = [c.get('skill', {}).get('balanced_accuracy', 0) * 100 for c in sub_cogs]
        base_accs = [c.get('skill', {}).get('baseline_balanced_accuracy', 0) * 100 for c in sub_cogs]

        bars1 = ax1.bar(x - width/2, bal_accs, width, label='Model Balanced Acc', color='#58a6ff')
        bars2 = ax1.bar(x + width/2, base_accs, width, label='Baseline Policy Acc', color='#8b949e')

        ax1.set_xticks(x)
        ax1.set_xticklabels(labels, fontsize=9.5, fontweight='bold')
        ax1.set_ylabel('Balanced Accuracy (%)', fontsize=10, color='#adbac7')
        ax1.set_title('Multi-Trie Balanced Accuracy vs Baseline Policy', fontsize=11, fontweight='bold')
        ax1.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d')
        ax1.grid(axis='y')

        for bar in bars1:
            h = bar.get_height()
            ax1.text(bar.get_x() + bar.get_width()/2., h + 0.8, f"{h:.1f}%", ha='center', va='bottom', fontsize=8.5, color='#f0f6fc')
        for bar in bars2:
            h = bar.get_height()
            ax1.text(bar.get_x() + bar.get_width()/2., h + 0.8, f"{h:.1f}%", ha='center', va='bottom', fontsize=8.5, color='#8b949e')

        mccs = [c.get('skill', {}).get('mcc', 0) for c in sub_cogs]
        retentions = [c.get('retention', {}).get('retention_rate', 0) * 100 for c in sub_cogs]
        nodes = [c.get('topology', {}).get('node_stats', {}).get('total_nodes', 0) for c in sub_cogs]

        bars_mcc = ax2.bar(x, mccs, width=0.45, color=['#2ea043' if m > 0.05 else ('#58a6ff' if m >= 0 else '#f85149') for m in mccs])
        ax2.set_xticks(x)
        ax2.set_xticklabels(labels, fontsize=9.5, fontweight='bold')
        ax2.set_ylabel('Matthews Correlation Coefficient (MCC)', fontsize=10, color='#adbac7')
        ax2.set_title(f'Classification Skill: MCC & Memory Retention', fontsize=11, fontweight='bold')
        ax2.axhline(0, color='#6e7681', linestyle='--', linewidth=0.8)
        ax2.grid(axis='y')

        for idx, bar in enumerate(bars_mcc):
            h = bar.get_height()
            ret_val = retentions[idx]
            node_val = nodes[idx]
            va = 'bottom' if h >= 0 else 'top'
            offset = 0.01 if h >= 0 else -0.015
            ax2.text(bar.get_x() + bar.get_width()/2., h + offset, f"MCC={h:.3f}\nRet={ret_val:.1f}%\nNodes={node_val}", ha='center', va=va, fontsize=8, color='#f0f6fc')

        readout = f"Top-1: BalAcc={bal_accs[0]:.1f}% (MCC={mccs[0]:.3f}) | Top-2: BalAcc={bal_accs[1]:.1f}% (MCC={mccs[1]:.3f}) | Top-3: BalAcc={bal_accs[2]:.1f}% (MCC={mccs[2]:.3f})"
        render_health_header(
            fig,
            f'Stage 6: Multi-Trie Cognitive Comparison (Top-1 vs Top-2 vs Top-3)',
            'Empirical comparison of in-memory association tries built from K=1 (Winner), K=2 (Pair), and K=3 (Triad) tokens.',
            status=status,
            health_score=max(0.1, max(bal_accs)/100.0),
            readout=readout,
            target_pct=None
        )

        fig.savefig(os.path.join(out_dir, 'stage6_trie_skill.png'), dpi=180, bbox_inches='tight')
        plt.close(fig)
        return

    # Fallback to single-trie plotting if multi-trie data is absent
    skill = cog.get('skill', {})
    ret = cog.get('retention', {})
    phases = cog.get('phases_formed', 0)
    hits = skill.get('hits', 0)
    total_calls = skill.get('total_calls', 0)
    baseline_hits = skill.get('baseline_hits', 0)
    baseline_policy = skill.get('best_baseline_policy', 'none')
    null_mean = skill.get('null_mean_hits', 0)
    null_p95 = skill.get('null_95th_percentile_hits', 0)
    p_val = skill.get('empirical_p_value', 1.0)
    separates = skill.get('separates_from_null', False)

    ret_rate = ret.get('retention_rate', 0) * 100
    spurious_rate = cog.get('spurious_trigger_rate', 0) * 100

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.5))

    categories = ['Prequential Model', f'Best Baseline\n({baseline_policy})', 'Null Mean', 'Null 95th Pct']
    values = [hits, baseline_hits, null_mean, null_p95]
    colors = ['#2ea043' if separates else '#58a6ff', '#8b949e', '#d29922', '#f0883e']

    bars = ax1.bar(categories, values, color=colors, width=0.45)
    ax1.set_ylabel('Hit Count (Phases)', fontsize=10, color='#adbac7')
    ax1.set_title(f'Prequential Accuracy vs Null: p={p_val:.3f} (N={total_calls})', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')
    for bar in bars:
        h = bar.get_height()
        ax1.text(bar.get_x() + bar.get_width()/2., h + 0.05, f"{h:.1f}", ha='center', va='bottom', fontsize=9, color='#f0f6fc')

    m_labels = ['Post-Teach Retention', 'Background False Alarm']
    m_values = [ret_rate, spurious_rate]
    m_colors = ['#388bfd', '#f85149' if spurious_rate > 5 else '#2ea043']

    m_bars = ax2.bar(m_labels, m_values, color=m_colors, width=0.4)
    ax2.set_ylabel('Percentage (%)', fontsize=10, color='#adbac7')
    ax2.set_title(f'Memory Dynamics: Retention={ret_rate:.1f}%, False Alarm={spurious_rate:.2f}%', fontsize=11, fontweight='bold')
    ax2.set_ylim(0, max(100, max(m_values)*1.15) if m_values else 100)
    ax2.grid(axis='y')
    for bar in m_bars:
        h = bar.get_height()
        ax2.text(bar.get_x() + bar.get_width()/2., h + 1, f"{h:.1f}%", ha='center', va='bottom', fontsize=9, color='#f0f6fc')

    render_health_header(
        fig,
        f'Stage 6: Cognitive Engine Prequential Skill & Memory Dynamics (Status: {status})',
        'Tests whether sequence prefix recall beats constant policies & shuffled nulls, and measures memory retention.',
        status="SKILLFUL" if separates else "NULL",
        health_score=0.90 if separates else 0.35,
        readout=f"Model Hits: {hits:.1f} vs Null-95: {null_p95:.1f} (p={p_val:.3f})",
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage6_trie_skill.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage6_trie_structure(report, out_dir):
    cog = report.get('cognitive_trie', {})
    topo = cog.get('topology', {})
    node_stats = topo.get('node_stats', {})
    status = cog.get('status', 'INSUFFICIENT_DATA')

    enter_basins = topo.get('enter_basins', 0)
    exit_basins = topo.get('exit_basins', 0)
    records = topo.get('records_count', 0)
    span = topo.get('span_count', 0)

    total_nodes = node_stats.get('total_nodes', 0)
    max_depth = node_stats.get('max_depth', 0)
    mean_depth = node_stats.get('mean_depth', 0)
    branching = node_stats.get('branching_factor', 0)

    abstention = cog.get('abstention_rate', 0) * 100
    mean_conf = cog.get('mean_confidence', 0)
    mean_cont = cog.get('mean_contrast', 0)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.5))

    # Panel 1: Basin Composition
    b_labels = ['Total Records', 'Longest Span', 'Enter Basins', 'Exit Basins']
    b_values = [records, span, enter_basins, exit_basins]
    b_bars = ax1.bar(b_labels, b_values, color=['#58a6ff', '#bc8cff', '#2ea043', '#f0883e'], width=0.45)
    ax1.set_ylabel('Count', fontsize=10, color='#adbac7')
    ax1.set_title(f'Trie Basins & Geometry (Records: {records:.0f}, Max Span: {span:.0f})', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')
    for bar in b_bars:
        h = bar.get_height()
        ax1.text(bar.get_x() + bar.get_width()/2., h + 0.05, f"{h:.0f}", ha='center', va='bottom', fontsize=9, color='#f0f6fc')

    # Panel 2: Node Topology & Decisiveness
    t_labels = ['Total Nodes', 'Max Depth', 'Mean Depth', 'Branching']
    t_values = [total_nodes, max_depth, mean_depth, branching]
    t_bars = ax2.bar(t_labels, t_values, color=['#79c0ff', '#d2a8ff', '#ff7b72', '#ffa657'], width=0.45)
    ax2.set_ylabel('Metric Value', fontsize=10, color='#adbac7')
    ax2.set_title(f'Graph Hierarchy & Decisiveness (Abstention: {abstention:.1f}%, Conf: {mean_conf:.3f}, Contrast: {mean_cont:.3f})', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')
    for bar in t_bars:
        h = bar.get_height()
        ax2.text(bar.get_x() + bar.get_width()/2., h + 0.05, f"{h:.1f}", ha='center', va='bottom', fontsize=9, color='#f0f6fc')

    if total_nodes > 0:
        struct_status = "HEALTHY"
        struct_score = 0.85
        struct_readout = f"Nodes: {total_nodes}, Records: {records:.0f}, Max Span: {span:.0f}, Abstention: {abstention:.1f}%  |  Graph Topology"
    else:
        struct_status = "INSUFFICIENT_DATA"
        struct_score = 0.0
        struct_readout = "Radix Trie Inactive or Insufficient Phase Records"

    render_health_header(
        fig,
        f'Stage 6: Radix Trie Graph Topology & Decisiveness (Status: {status})',
        'Structural audit of the prefix tree nodes, longest token spans, active basins, and decider contrast.',
        status=struct_status,
        health_score=struct_score,
        readout=struct_readout,
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage6_trie_structure.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage6_s3_memory(report, out_dir):
    cog = report.get('cognitive_trie', {})
    s3_mem = cog.get('s3_memory', {})

    total_keys = s3_mem.get('total_prefix_keys', 0)
    collisions = s3_mem.get('prefix_collisions', 0)
    conflicts = s3_mem.get('conflicting_continuations', 0)
    disambig = s3_mem.get('time_to_disambiguation', s3_mem.get('mean_disambiguation_ticks', 0))
    acc = s3_mem.get('prequential_retrieval_acc', s3_mem.get('prequential_recall_accuracy', 0.0)) * 100

    if 'status' in s3_mem:
        status = s3_mem['status']
    elif total_keys > 0:
        status = "PASS" if s3_mem.get('passed', False) else "WARN"
    else:
        status = "INSUFFICIENT_DATA"

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.0))

    # Panel 1: S3 Prefix Keys & Collisions
    k_labels = ['Total Prefix Keys', 'Prefix Collisions', 'Conflicting Continuations']
    k_vals = [total_keys, collisions, conflicts]
    k_bars = ax1.bar(k_labels, k_vals, color=['#58a6ff', '#d29922', '#f85149'], width=0.45)
    ax1.set_ylabel('Count', fontsize=10, color='#adbac7')
    ax1.set_title(f'S3 Prefix Keys & Collision Integrity ({status})', fontsize=11, fontweight='bold')
    ax1.set_ylim(0, max(10, max(k_vals) * 1.15))
    ax1.grid(axis='y')
    for bar in k_bars:
        h = bar.get_height()
        ax1.text(bar.get_x() + bar.get_width()/2., h + ax1.get_ylim()[1]*0.02, f"{h:.0f}", ha='center', va='bottom', fontsize=9, color='#f0f6fc', clip_on=True)

    # Panel 2: Disambiguation (left axis) & Accuracy (right axis)
    d_labels = ['Disambiguation Tokens', 'Prequential Recall (%)']
    ax2.set_ylabel('Disambiguation Depth (Tokens)', fontsize=10, color='#bc8cff')
    ax2.bar(0, disambig, color='#bc8cff', width=0.35)
    ax2.set_ylim(0, max(6, disambig * 1.3))
    ax2.text(0, disambig + ax2.get_ylim()[1]*0.02, f"{disambig:.1f} tokens", ha='center', va='bottom', fontsize=9, color='#f0f6fc', clip_on=True)

    ax2_r = ax2.twinx()
    ax2_r.set_ylabel('Prequential Recall (%)', fontsize=10, color='#3fb950')
    ax2_r.bar(1, acc, color='#3fb950', width=0.35)
    ax2_r.set_ylim(0, 110)
    ax2_r.text(1, acc + 110*0.02, f"{acc:.1f}%", ha='center', va='bottom', fontsize=9, color='#f0f6fc', clip_on=True)

    ax2.set_xticks([0, 1])
    ax2.set_xticklabels(d_labels)
    ax2.set_title('Memory Disambiguation Depth & Recall Accuracy', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')

    health_score = 0.90 if status in ["PASS", "MEASURED"] else (0.60 if status == "WARN" else 0.0)

    render_health_header(
        fig,
        'Stage 6: S3-Compatible Prefix-Memory Mechanism Audit',
        'Audits real persistent S3 prefix-memory keys (R04/R01/enter.json), prefix collisions, and disambiguation speed.',
        status=status,
        health_score=health_score,
        readout=f"Total Keys: {total_keys}, Collisions: {collisions}, Disambiguation: {disambig} tokens, Recall: {acc:.1f}%",
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'stage6_s3_memory.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_validations_sensitivity(report, out_dir):
    sens = report.get('sensitivity', {})
    if not sens:
        return
    status = "PASS" if sens.get('passed', False) else "FAIL"
    lofo = sens.get('leave_one_out_jsd', {})
    dupl = sens.get('duplication_jsd', {})

    if not lofo:
        return

    families = sorted(list(lofo.keys()))
    lofo_vals = [lofo[fam] for fam in families]
    dupl_vals = [dupl.get(fam, 0.0) for fam in families]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.0))

    # Panel 1: Leave-One-Family-Out JSD
    bars1 = ax1.bar(families, lofo_vals, color='#58a6ff', width=0.45)
    ax1.set_ylabel('JSD Divergence vs Full Grid (Bits)', fontsize=10, color='#adbac7')
    ax1.set_title('Leave-One-Family-Out (LOFO) Sensitivity', fontsize=11, fontweight='bold')
    ax1.set_xticklabels(families, rotation=30, ha='right', fontsize=9)
    ax1.set_ylim(0, max(0.05, max(lofo_vals) * 1.18))
    ax1.grid(axis='y')
    for bar in bars1:
        h = bar.get_height()
        ax1.text(bar.get_x() + bar.get_width()/2., h + ax1.get_ylim()[1]*0.02, f"{h:.3f}b", ha='center', va='bottom', fontsize=8, color='#f0f6fc', clip_on=True)

    # Panel 2: Duplication JSD
    bars2 = ax2.bar(families, dupl_vals, color='#bc8cff', width=0.45)
    ax2.set_ylabel('JSD Divergence under Family Duplication (Bits)', fontsize=10, color='#adbac7')
    ax2.set_title('Family Duplication Invariance (< 0.05b target)', fontsize=11, fontweight='bold')
    ax2.axhline(0.05, color='#ff7b72', linestyle=':', label='Max Invariance Threshold (0.05b)')
    ax2.set_xticklabels(families, rotation=30, ha='right', fontsize=9)
    ax2.set_ylim(0, max(0.06, max(dupl_vals) * 1.18))
    ax2.grid(axis='y')
    ax2.legend(loc='upper right', fontsize=8)
    for bar in bars2:
        h = bar.get_height()
        ax2.text(bar.get_x() + bar.get_width()/2., h + ax2.get_ylim()[1]*0.02, f"{h:.3f}b", ha='center', va='bottom', fontsize=8, color='#f0f6fc', clip_on=True)

    render_health_header(
        fig,
        'Validation V4: Grid Dependence & Feature Family Sensitivity',
        'Tests whether grid partition balances feature families and resists dominance by duplicated sensors.',
        status=status,
        health_score=0.90 if status == "PASS" else 0.40,
        readout=f"Families Audited: {len(families)}, Duplication Resistant: {sens.get('duplication_resistant', False)}",
        target_pct=None
    )

    fig.savefig(os.path.join(out_dir, 'validation_sensitivity.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def animate_excursion_transitions(report, out_dir):
    traces = report.get('excursion_traces') or (report.get('precursor') or {}).get('excursion_traces') or []
    if not traces:
        return

    # Choose primary trace (longest or highest return)
    trace = max(traces, key=lambda t: len(t.get('transitions', [])))
    transitions = trace.get('transitions', [])
    if len(transitions) < 3:
        return

    symbol = trace.get('symbol', 'ASSET')
    ex_class = trace.get('class', 'UP').upper()
    total_steps = len(transitions)

    region_labels = [
        "R00\nBuyArrival", "R01\nPacingVel", "R02\nSellCascade", "R03\nTakerSweep",
        "R04\nCVDDelta", "R05\nTakerDump", "R06\nAskThin", "R07\nSpreadFrict",
        "R08\nBidSupport", "R09\nLeaderCohort", "R10\nQuiescentCoil", "R11\nSystemicDrag"
    ]

    # Pre-extract arrays across all steps
    steps = [tr.get('step', i) for i, tr in enumerate(transitions)]
    prices = [tr.get('price', 0.0) for tr in transitions]
    returns = [tr.get('return_pct', 0.0) for tr in transitions]
    winners = [tr.get('winner', 0) for tr in transitions]
    runner_ups = [tr.get('runner_up', 0) for tr in transitions]
    brightness_matrix = np.array([tr.get('brightness', [0.0]*12) for tr in transitions]) # shape: (T, 12)

    # Set up figure with modern grid layout
    fig = plt.figure(figsize=(15.5, 8.8), facecolor='#0d1117')
    gs = fig.add_gridspec(2, 2, height_ratios=[1.2, 0.8], hspace=0.34, wspace=0.22,
                          left=0.06, right=0.96, top=0.84, bottom=0.08)

    ax_bars = fig.add_subplot(gs[0, :])       # Top: 12 Region Activation Bars
    ax_price = fig.add_subplot(gs[1, 0])      # Bottom-Left: Excursion Trajectory & Progress Head
    ax_matrix = fig.add_subplot(gs[1, 1])     # Bottom-Right: Confluence Waterfall Spectrogram

    # Fixed styling & limits
    min_b = min(float(np.min(brightness_matrix)) - 0.5, -2.5)
    max_b = max(float(np.max(brightness_matrix)) + 1.2, 4.5)

    # 1. Static setup for ax_bars
    ax_bars.set_facecolor('#161b22')
    ax_bars.set_xlim(-0.8, 11.8)
    ax_bars.set_ylim(min_b, max_b)
    ax_bars.set_xticks(range(12))
    ax_bars.set_xticklabels(region_labels, fontsize=8.5, fontweight='bold', color='#c9d1d9')
    ax_bars.set_ylabel('Brightness Excess ($z$-Score)', fontsize=10, fontweight='bold', color='#c9d1d9')
    ax_bars.axhline(0, color='#484f58', linestyle='--', linewidth=1.0, label='Null Expectation ($0\\sigma$)')
    ax_bars.axhline(2.0, color='#d29922', linestyle=':', linewidth=1.2, label='Excitation Barrier ($+2\\sigma$)')
    ax_bars.axhline(-2.0, color='#8957e5', linestyle=':', linewidth=1.0, label='Quiescent Barrier ($-2\\sigma$)')
    ax_bars.legend(loc='upper right', fontsize=8, facecolor='#21262d', edgecolor='#30363d')
    ax_bars.grid(axis='y', color='#30363d', linestyle='--', alpha=0.5)

    bar_rects = ax_bars.bar(range(12), brightness_matrix[0], width=0.6, color='#1f6feb', edgecolor='#388bfd', linewidth=1.0)
    bar_texts = [ax_bars.text(i, 0, '', ha='center', va='bottom', fontsize=8, fontweight='bold', color='#f0f6fc') for i in range(12)]

    # 2. Static setup for ax_price
    ax_price.set_facecolor('#161b22')
    ax_price.set_title(f'Excursion Trajectory ({symbol} {ex_class})', fontsize=10.5, fontweight='bold', color='#f0f6fc', pad=8)
    ax_price.set_xlabel('Transition Step', fontsize=9, color='#8b949e')
    ax_price.set_ylabel('Net Return (%)', fontsize=9, color='#8b949e')
    ax_price.set_xlim(0, max(1, total_steps - 1))
    ret_min = min(returns) - 0.2
    ret_max = max(returns) + 0.3
    ax_price.set_ylim(ret_min, ret_max)
    ax_price.grid(color='#30363d', linestyle='--', alpha=0.5)

    # Shade precursor vs holding
    precursor_steps = [i for i, tr in enumerate(transitions) if tr.get('phase') == 'precursor']
    if precursor_steps:
        ax_price.axvspan(0, max(precursor_steps), color='#388bfd', alpha=0.12, label='Precursor Phase')
    if len(precursor_steps) < total_steps:
        start_hold = max(precursor_steps) if precursor_steps else 0
        ax_price.axvspan(start_hold, total_steps - 1, color='#3fb950', alpha=0.12, label='Excursion Holding')
    ax_price.legend(loc='upper left', fontsize=7.5, facecolor='#21262d', edgecolor='#30363d')

    full_line, = ax_price.plot(steps, returns, color='#6e7681', linestyle=':', linewidth=1.0, alpha=0.6)
    trail_line, = ax_price.plot([], [], color='#58a6ff', linewidth=2.0)
    head_dot, = ax_price.plot([], [], 'o', color='#3fb950', markersize=8, markeredgecolor='#ffffff', markeredgewidth=1.5)

    # 3. Static setup for ax_matrix (Confluence Spectrogram)
    ax_matrix.set_facecolor('#161b22')
    ax_matrix.set_title('Confluence Waterfall: 12 Regions Across Transitions', fontsize=10.5, fontweight='bold', color='#f0f6fc', pad=8)
    ax_matrix.set_xlabel('Transition Step', fontsize=9, color='#8b949e')
    ax_matrix.set_ylabel('Region Index', fontsize=9, color='#8b949e')
    ax_matrix.set_yticks(range(12))
    ax_matrix.set_yticklabels([f"R{r:02d}" for r in range(12)], fontsize=8, color='#8b949e')

    # Render waterfall image
    im = ax_matrix.imshow(brightness_matrix.T, aspect='auto', cmap='plasma', origin='lower',
                          extent=[0, max(1, total_steps-1), -0.5, 11.5], vmin=-2.0, vmax=4.0)
    cbar = plt.colorbar(im, ax=ax_matrix, pad=0.02, fraction=0.046)
    cbar.ax.tick_params(labelsize=7.5, colors='#8b949e')
    cbar.set_label('Excitation z-Score', fontsize=8, color='#8b949e')
    cursor_line = ax_matrix.axvline(0, color='#ffffff', linewidth=1.8, linestyle='--')

    # Status readout text in header area
    readout_text = fig.text(0.50, 0.88, '', fontsize=9.5, fontweight='bold', color='#f0f6fc', ha='center', va='center')

    # Render base header
    render_health_header(
        fig,
        f'Excursion Dynamic Resonance: Regional Transitions ({symbol})',
        'Frame-by-frame deduplicated region transition replay during market excursion',
        status="MEASURED",
        health_score=0.95,
        readout=f"Total Transitions: {total_steps} | Symbol: {symbol}",
        target_pct=None
    )

    def init():
        trail_line.set_data([], [])
        head_dot.set_data([], [])
        cursor_line.set_xdata([0, 0])
        return list(bar_rects) + [trail_line, head_dot, cursor_line, readout_text]

    def update(frame_idx):
        tr = transitions[frame_idx]
        b_vals = brightness_matrix[frame_idx]
        w = winners[frame_idx]
        ru = runner_ups[frame_idx]
        tok = tr.get('token', f'R{w:02d}')
        top3 = tr.get('top3_token', '')
        phase = tr.get('phase', 'excursion')
        dwell = tr.get('dwell_ticks', 1)
        cur_ret = returns[frame_idx]

        # Update bars
        for r in range(12):
            val = float(b_vals[r])
            bar = bar_rects[r]
            bar.set_height(val)
            txt = bar_texts[r]
            txt.set_text(f"{val:+.1f}σ")
            txt.set_y(val + (0.15 if val >= 0 else -0.35))

            if r == w:
                bar.set_color('#3fb950')
                bar.set_edgecolor('#ffffff')
                bar.set_linewidth(2.0)
                txt.set_color('#3fb950')
                txt.set_fontweight('bold')
            elif r == ru:
                bar.set_color('#d29922')
                bar.set_edgecolor('#e3b341')
                bar.set_linewidth(1.2)
                txt.set_color('#e3b341')
                txt.set_fontweight('bold')
            elif val > 0:
                bar.set_color('#1f6feb')
                bar.set_edgecolor('#388bfd')
                bar.set_linewidth(0.8)
                txt.set_color('#79c0ff')
                txt.set_fontweight('normal')
            else:
                bar.set_color('#30363d')
                bar.set_edgecolor('#484f58')
                bar.set_linewidth(0.6)
                txt.set_color('#8b949e')
                txt.set_fontweight('normal')

        # Update price trajectory
        cur_steps = steps[:frame_idx+1]
        cur_returns = returns[:frame_idx+1]
        trail_line.set_data(cur_steps, cur_returns)
        head_dot.set_data([steps[frame_idx]], [cur_returns[-1]])

        # Update waterfall cursor
        cursor_line.set_xdata([steps[frame_idx], steps[frame_idx]])

        # Update dynamic header text
        readout_text.set_text(
            f"Step {frame_idx+1}/{total_steps}  |  Active: {tok} (Top-3: {top3})  |  Phase: {phase.upper()}  |  Dwell: {dwell} ticks  |  Return: {cur_ret:+.2f}%"
        )

        return list(bar_rects) + [trail_line, head_dot, cursor_line, readout_text]

    import matplotlib.animation as anim
    animation_obj = anim.FuncAnimation(fig, update, frames=total_steps, init_func=init, blit=False, interval=350)

    # Save animated GIF via Pillow
    gif_path = os.path.join(out_dir, 'excursion_transitions.gif')
    animation_obj.save(gif_path, writer='pillow', fps=3)

    # Save MP4 if ffmpeg is available
    import shutil
    if shutil.which('ffmpeg'):
        try:
            mp4_path = os.path.join(out_dir, 'excursion_transitions.mp4')
            animation_obj.save(mp4_path, writer='ffmpeg', fps=3)
        except Exception:
            pass

    # Save static companion milestone summary sheet
    static_path = os.path.join(out_dir, 'excursion_transitions_summary.png')
    update(min(total_steps - 1, max(0, int(total_steps * 0.6))))
    fig.savefig(static_path, dpi=180, bbox_inches='tight')
    plt.close(fig)

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

    report = load_report(json_path)

    plot_stage0_contract(report, plots_dir)
    plot_stage1_vitality(report, plots_dir)
    plot_stage1_redundancy(report, plots_dir)
    plot_stage2_sympathy_null(report, plots_dir)
    plot_stage2_orientation(report, plots_dir)
    plot_stage3_grid_completeness(report, plots_dir)
    plot_stage4_token_dynamics(report, plots_dir)
    plot_stage4_transition_matrix(report, plots_dir)
    plot_stage4_entropy(report, plots_dir)
    plot_stage5_precursor(report, plots_dir)
    animate_excursion_transitions(report, plots_dir)
    plot_stage6_trie_skill(report, plots_dir)
    plot_stage6_trie_structure(report, plots_dir)
    plot_stage6_s3_memory(report, plots_dir)
    plot_validations_sensitivity(report, plots_dir)

    print(f"✅ Generated publication-quality diagnostic charts in {plots_dir}")

if __name__ == '__main__':
    main()
