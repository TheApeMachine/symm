#!/usr/bin/env python3
"""
SYMM Audit Visualization Engine
Reads audit_results.json and produces intuitive, publication-quality diagnostic charts
with embedded plain-English explanations underneath titles (never obscuring data)
and complete, untruncated tick labels.
"""

import sys
import os
import json
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import numpy as np

# Set dark modern styling
plt.style.use('dark_background')
plt.rcParams['font.sans-serif'] = 'Helvetica, Arial, DejaVu Sans'
plt.rcParams['axes.edgecolor'] = '#444c56'
plt.rcParams['axes.linewidth'] = 0.8
plt.rcParams['grid.color'] = '#2d333b'
plt.rcParams['grid.linestyle'] = '--'
plt.rcParams['grid.alpha'] = 0.6

def load_report(json_path):
    with open(json_path, 'r') as f:
        return json.load(f)

def add_header(fig, ax, main_title, meaning, good_criteria, higher_lower, pad=48):
    """
    Renders the main title and an explanation banner card positioned directly
    underneath the title, above the plot axes, ensuring data is NEVER obscured.
    """
    ax.set_title(main_title, fontsize=12.5, fontweight='bold', pad=pad, color='#f0f6fc')
    
    explanation = (
        f"[WHAT THIS MEANS]: {meaning}\n"
        f"[WHAT GOOD LOOKS LIKE]: {good_criteria}   |   [SCALE]: {higher_lower}"
    )
    
    ax.text(
        0.5, 1.025,
        explanation,
        transform=ax.transAxes,
        ha='center', va='bottom',
        fontsize=8.0,
        color='#adbac7',
        bbox=dict(boxstyle='round,pad=0.5', facecolor='#1c2128', edgecolor='#373e47', alpha=0.95)
    )

def add_figure_header(fig, main_title, meaning, good_criteria, higher_lower):
    """
    Used for multi-subplot figures. Puts title and explanation card at top of figure.
    """
    fig.suptitle(main_title, fontsize=13, fontweight='bold', color='#f0f6fc', y=0.985)
    explanation = (
        f"[WHAT THIS MEANS]: {meaning}\n"
        f"[WHAT GOOD LOOKS LIKE]: {good_criteria}   |   [SCALE]: {higher_lower}"
    )
    fig.text(
        0.5, 0.915,
        explanation,
        ha='center', va='top',
        fontsize=8.0,
        color='#adbac7',
        bbox=dict(boxstyle='round,pad=0.5', facecolor='#1c2128', edgecolor='#373e47', alpha=0.95)
    )
    fig.subplots_adjust(top=0.82)

def plot_stage1_vitality(report, out_dir):
    metrics = report.get('vitality', {}).get('metrics', [])
    if not metrics:
        return

    # Sort by coverage descending, show top 25 with full names
    sorted_metrics = sorted(metrics, key=lambda m: m.get('coverage', 0), reverse=True)[:25]
    names = [m['name'] for m in sorted_metrics]
    coverages = [m.get('coverage', 0) * 100 for m in sorted_metrics]
    statuses = [m.get('status', 'HEALTHY') for m in sorted_metrics]

    colors = []
    for s in statuses:
        if s == 'HEALTHY':
            colors.append('#2ea043') # Green
        elif s == 'SPORADIC':
            colors.append('#d29922') # Yellow
        else:
            colors.append('#f85149') # Red

    fig, ax = plt.subplots(figsize=(13, 8))
    bars = ax.barh(range(len(names)), coverages, color=colors, height=0.65, edgecolor='#22272e')
    ax.set_yticks(range(len(names)))
    ax.set_yticklabels(names, fontsize=8)
    ax.invert_yaxis()
    ax.set_xlabel('Tick Coverage (%)', fontsize=10, color='#adbac7')
    ax.axvline(80, color='#2ea043', linestyle=':', alpha=0.7, label='Optimal Threshold (80%)')
    ax.axvline(20, color='#d29922', linestyle=':', alpha=0.7, label='Sporadic Boundary (20%)')
    ax.set_xlim(0, 105)
    ax.grid(axis='x')
    ax.tick_params(axis='y', labelsize=8)

    add_header(
        fig, ax,
        'Stage 1: Top Metric Presence & Vitality (Coverage % of Ticks)',
        'Percentage of market ticks in which this sensor emitted an observation.',
        'Green (>80%): Continuous evidence. Red/Yellow (<20%): Sporadic drops or dead constant.',
        'HIGHER IS BETTER (Target: > 50-80% coverage).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage1_metric_vitality.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage1_redundancy(report, out_dir):
    redundant = report.get('vitality', {}).get('redundant_pairs', [])
    pairs = [f"{p['metric_a']} <-> {p['metric_b']}" for p in redundant[:15]]
    corrs = [p['correlation'] for p in redundant[:15]]

    fig_height = max(6.0, len(pairs) * 0.45 + 2.5) if pairs else 6.0
    fig, ax = plt.subplots(figsize=(14, fig_height))

    if not redundant:
        ax.text(0.5, 0.5, "[OK] Zero Redundant Clones Found (|r| >= 0.95)\nEvery healthy metric represents an independent degree of freedom.",
                ha='center', va='center', fontsize=12, color='#2ea043', transform=ax.transAxes)
    else:
        ax.barh(range(len(pairs)), corrs, color='#f85149', height=0.55)
        ax.set_yticks(range(len(pairs)))
        ax.set_yticklabels(pairs, fontsize=8)
        ax.set_xlabel('Pearson Correlation Coefficient (r)', fontsize=10, color='#adbac7')
        ax.set_xlim(0.9, 1.02)
        ax.invert_yaxis()
        ax.tick_params(axis='y', labelsize=8)

    add_header(
        fig, ax,
        'Stage 1: Metric Redundancy & Multicollinearity (|r| >= 0.95)',
        'Pairs of metrics that track each other so closely they represent duplicate information.',
        'Empty/Few pairs: Every metric provides distinct statistical information.',
        'FEWER IS BETTER (Red bars indicate metrics that can be pruned).'
    )

    fig.tight_layout()
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
    ax.plot(null_centers, null_p, color='#8b949e', linewidth=2.2, label='Shuffled Null Distribution (Pure Noise)', alpha=0.9)
    ax.fill_between(null_centers, null_p, color='#8b949e', alpha=0.25)

    ax.plot(real_centers, real_p, color='#58a6ff', linewidth=2.5, label='Real Market Concordance', marker='o', markersize=4)
    ax.fill_between(real_centers, real_p, color='#58a6ff', alpha=0.3)

    p95 = null_dist.get('percentile_95', 0)
    if p95 > 0:
        ax.axvline(p95, color='#d29922', linestyle='--', linewidth=1.5, label=f'95th Percentile Null Bound ({p95:.2f})')

    ax.set_xlabel('Pairwise Concordance / Sympathy Score', fontsize=10, color='#adbac7')
    ax.set_ylabel('Probability Density', fontsize=10, color='#adbac7')
    ax.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
    ax.grid(True)

    sep_ratio = symp.get('separation_ratio', 0) * 100
    ks = symp.get('ks_statistic', 0)

    add_header(
        fig, ax,
        'Stage 2: Pair Sympathy vs. Shuffled Null (Permutation Test)',
        'Compares true pair relationships against data where time order is deliberately scrambled.',
        f'Real (Blue) separates with fat tails outside Null (Gray). Separation: {sep_ratio:.1f}%, KS: {ks:.3f}.',
        'HIGHER SEPARATION IS BETTER (Overlap indicates relationships are random noise).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage2_sympathy_null.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage2_orientation(report, out_dir):
    symp = report.get('sympathy', {})
    pos = symp.get('positive_pairs', 0)
    inv = symp.get('inverse_pairs', 0)

    fig, ax = plt.subplots(figsize=(8.5, 6))
    bars = ax.bar(['Direct Sympathy (+)', 'Inverse Sympathy (-)'], [pos, inv], color=['#2ea043', '#a371f7'], width=0.45)
    ax.set_ylabel('Number of Active Metric Pairs', fontsize=10, color='#adbac7')
    ax.grid(axis='y')

    for bar in bars:
        yval = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2, yval + 1, f"{int(yval)}", ha='center', va='bottom', fontsize=10, fontweight='bold', color='#f0f6fc')

    add_header(
        fig, ax,
        'Stage 2: Directional Symmetry of Sympathy',
        'Financial markets naturally contain inverse movements (e.g. risk-on vs risk-off).',
        'Both direct (+) and inverse (-) relationships should be present and recognized.',
        'BALANCED IS BETTER (If inverse is near zero, inverse structure is being discarded).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage2_orientation_balance.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage3_grid_stability(report, out_dir):
    grid_stab = report.get('grid_stability', {})
    grid_a = grid_stab.get('grid_a', {})
    grid_b = grid_stab.get('grid_b', {})
    ari = grid_stab.get('adjusted_rand_idx', 0)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))

    sizes_a = sorted(list(grid_a.get('region_sizes', {}).values()), reverse=True)
    sizes_b = sorted(list(grid_b.get('region_sizes', {}).values()), reverse=True)

    ax1.plot(range(1, len(sizes_a)+1), sizes_a, marker='o', color='#58a6ff', label=f"Early Grid ({len(sizes_a)} regions)")
    ax1.plot(range(1, len(sizes_b)+1), sizes_b, marker='s', color='#f0883e', label=f"Late Grid ({len(sizes_b)} regions)")
    ax1.set_xlabel('Region Rank (Largest to Smallest)', fontsize=9, color='#adbac7')
    ax1.set_ylabel('Cell Count in Region', fontsize=9, color='#adbac7')
    ax1.set_title('Region Size Distribution (Balance vs Collapse)', fontsize=11, fontweight='bold')
    ax1.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8)
    ax1.grid(True)

    ari_color = '#2ea043' if ari >= 0.30 else ('#d29922' if ari >= 0.10 else '#f85149')
    ax2.bar(['Adjusted Rand Index (ARI)'], [ari], color=ari_color, width=0.35)
    ax2.axhline(0.30, color='#2ea043', linestyle='--', label='Stability Benchmark (0.30)')
    ax2.axhline(0.00, color='#8b949e', linestyle=':', label='Random Partition (0.00)')
    ax2.set_ylim(-0.1, 1.05)
    ax2.set_ylabel('Agreement Score', fontsize=9, color='#adbac7')
    ax2.set_title(f'Temporal Agreement: ARI = {ari:.3f}', fontsize=11, fontweight='bold')
    ax2.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8)
    ax2.grid(axis='y')

    add_figure_header(
        fig,
        'Stage 3: Grid Partition Stability Across Disjoint Time Windows',
        'Measures whether two grids independently developed from disjoint time periods group the same metrics.',
        'ARI > 0.30 indicates reproducible friendships. Smooth curves indicate balanced regions (no giant bucket).',
        'HIGHER ARI IS BETTER (1.0 = identical partitions; 0.0 = pure random re-clustering).'
    )

    fig.savefig(os.path.join(out_dir, 'stage3_region_partitioning.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_token_dynamics(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    freqs = dynamics.get('token_frequencies', {})

    if not freqs:
        return

    sorted_tokens = sorted(freqs.items(), key=lambda item: item[1], reverse=True)[:15]
    tok_names = [t[0] for t in sorted_tokens]
    tok_counts = [t[1] for t in sorted_tokens]

    fig, ax = plt.subplots(figsize=(12, 6.5))
    bars = ax.bar(tok_names, tok_counts, color='#388bfd', width=0.55, edgecolor='#1f6feb')
    ax.set_ylabel('Emission Count', fontsize=10, color='#adbac7')
    ax.set_xlabel('Region Token', fontsize=10, color='#adbac7')
    ax.grid(axis='y')
    ax.tick_params(axis='x', labelsize=8.5)

    max_dom = dynamics.get('max_token_dominance', 0) * 100
    for bar in bars:
        yval = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2, yval + 1, f"{int(yval)}", ha='center', va='bottom', fontsize=8, color='#f0f6fc')

    add_header(
        fig, ax,
        'Stage 4: Token Emission Frequency & Dominance',
        'Distribution of region tokens emitted as market observations light the grid.',
        f'Healthy: Well-distributed across multiple regions. Max token dominance is {max_dom:.1f}%.',
        'LOWER DOMINANCE IS BETTER (Dominance > 80% represents state collapse into a single region).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_token_dynamics.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_transition_matrix(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    transitions = dynamics.get('transitions', {})
    if not transitions:
        return

    active_tokens = sorted(list(transitions.keys()))[:12]
    matrix = np.zeros((len(active_tokens), len(active_tokens)))

    for i, from_tok in enumerate(active_tokens):
        row_total = sum(transitions[from_tok].values())
        if row_total == 0:
            continue
        for j, to_tok in enumerate(active_tokens):
            matrix[i, j] = transitions[from_tok].get(to_tok, 0) / row_total

    fig, ax = plt.subplots(figsize=(9.5, 8.5))
    cax = ax.imshow(matrix, cmap='magma', interpolation='nearest', vmin=0, vmax=1.0)
    fig.colorbar(cax, ax=ax, shrink=0.8, label='Transition Probability P(To | From)')

    ax.set_xticks(range(len(active_tokens)))
    ax.set_yticks(range(len(active_tokens)))
    ax.set_xticklabels(active_tokens, fontsize=8.5, rotation=45)
    ax.set_yticklabels(active_tokens, fontsize=8.5)
    ax.set_xlabel('Next Region Token (t)', fontsize=10, color='#adbac7')
    ax.set_ylabel('Prior Region Token (t-1)', fontsize=10, color='#adbac7')

    add_header(
        fig, ax,
        'Stage 4: Markov Token Transition Probability Matrix',
        'Probability of moving from Region Token i to Region Token j on consecutive ticks.',
        'Bright diagonal = persistence (dwell time). Structured off-diagonal blocks = predictable regime shifts.',
        'ORGANIZED STRUCTURE IS BETTER (Uniform noise represents an unpredictable slot machine).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_transition_matrix.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_entropy(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    real_h = dynamics.get('transition_entropy', 0)
    null_h = dynamics.get('null_transition_entropy', 0)
    red_bits = dynamics.get('entropy_reduction_bits', 0)

    fig, ax = plt.subplots(figsize=(8.5, 6))
    bars = ax.bar(['Real Transition Entropy', 'Shuffled Null Entropy'], [real_h, null_h], color=['#58a6ff', '#8b949e'], width=0.45)
    ax.set_ylabel('Entropy (Bits / Transition)', fontsize=10, color='#adbac7')
    ax.grid(axis='y')

    for bar in bars:
        yval = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2, yval + 0.05, f"{yval:.3f} bits", ha='center', va='bottom', fontsize=9, fontweight='bold', color='#f0f6fc')

    add_header(
        fig, ax,
        'Stage 4: Transition Predictability vs. Random Shuffled Null',
        'Measures how unpredictable the next region token is given the current token.',
        f'Real entropy is {real_h:.3f} bits vs Null {null_h:.3f} bits (Entropy Reduction: {red_bits:.3f} bits).',
        'LOWER REAL ENTROPY IS BETTER (Indicates genuine predictive temporal sequence structure).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_transition_entropy.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage5_precursor(report, out_dir):
    prec = report.get('precursor', {})
    jsd = prec.get('precursor_divergence', 0)
    null_95 = prec.get('null_divergence_95', 0)
    null_mean = prec.get('null_divergence_mean', 0)
    sep_ratio = prec.get('separation_ratio', 1.0)
    prec_tokens = prec.get('precursor_tokens', {})
    bg_tokens = prec.get('background_tokens', {})
    det_found = prec.get('detections_found', 0)
    classes = prec.get('excursions_found', [])
    prec_ticks = prec.get('precursor_ticks', 0)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6.5))

    # Left: Divergence vs Null
    div_color = '#2ea043' if jsd >= null_95 and jsd > 0 else '#d29922'
    ax1.bar(['Real Precursor JSD', 'Null Shuffled Mean'], [jsd, null_mean], color=[div_color, '#8b949e'], width=0.45)
    if null_95 > 0:
        ax1.axhline(null_95, color='#f85149', linestyle='--', linewidth=1.5, label=f'95th Percentile Null Bound ({null_95:.3f} bits)')
        ax1.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
    ax1.set_ylabel('Divergence (Bits)', fontsize=10, color='#adbac7')
    ax1.set_title(f'Precursor Divergence vs Empirical Null (Ratio: {sep_ratio:.2f}x)', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')

    # Right: Precursor Token Frequency vs Background
    if prec_tokens and sum(prec_tokens.values()) > 0:
        top_tokens = sorted(prec_tokens.keys(), key=lambda k: prec_tokens[k], reverse=True)[:8]
        total_p = sum(prec_tokens.values())
        total_b = sum(bg_tokens.values()) if sum(bg_tokens.values()) > 0 else 1
        p_pcts = [prec_tokens[k] / total_p * 100 for k in top_tokens]
        b_pcts = [bg_tokens.get(k, 0) / total_b * 100 for k in top_tokens]

        x = np.arange(len(top_tokens))
        width = 0.35
        ax2.bar(x - width/2, p_pcts, width, label='Precursor Window', color='#f0883e')
        ax2.bar(x + width/2, b_pcts, width, label='Background Baseline', color='#58a6ff', alpha=0.7)
        ax2.set_xticks(x)
        ax2.set_xticklabels(top_tokens, fontsize=8.5)
        ax2.set_ylabel('Emission Frequency (%)', fontsize=10, color='#adbac7')
        ax2.set_title(f'Precursor Token Enrichment ({prec_ticks} Precursor Ticks)', fontsize=11, fontweight='bold')
        ax2.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
        ax2.grid(axis='y')
    else:
        status_text = (
            f"Excursions Detected In-Memory: {det_found} ({', '.join(classes) if classes else 'none'})\n"
            f"Precursor Ticks in Sample: {prec_ticks}\n\n"
            "Precursor windows fall outside the initial sampled tick window.\n"
            "Increase --ticks to cover the full precursor horizon."
        )
        ax2.text(0.5, 0.5, status_text, ha='center', va='center', fontsize=10, color='#adbac7',
                 bbox=dict(boxstyle='round,pad=0.8', facecolor='#1c2128', edgecolor='#373e47'))
        ax2.set_axis_off()

    add_figure_header(
        fig,
        'Stage 5: Precursor Informativeness & Null Divergence Separation',
        'Tests whether sensory/grid states preceding price ignition moves are statistically distinguishable from ambient market noise.',
        f'{det_found} excursions detected in-memory. Separation ratio = {sep_ratio:.2f}x vs 95th percentile null.',
        'HIGHER RATIO IS BETTER (Ratio > 1.0x confirms genuine precursor signal exceeding ambient noise).'
    )

    fig.savefig(os.path.join(out_dir, 'stage5_precursor_separation.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def main():
    if len(sys.argv) < 3:
        print("Usage: plot_audit.py <audit_results.json> <output_dir>")
        sys.exit(1)

    json_path = sys.argv[1]
    out_dir = sys.argv[2]
    plots_dir = os.path.join(out_dir, 'plots')
    os.makedirs(plots_dir, exist_ok=True)

    report = load_report(json_path)

    plot_stage1_vitality(report, plots_dir)
    plot_stage1_redundancy(report, plots_dir)
    plot_stage2_sympathy_null(report, plots_dir)
    plot_stage2_orientation(report, plots_dir)
    plot_stage3_grid_stability(report, plots_dir)
    plot_stage4_token_dynamics(report, plots_dir)
    plot_stage4_transition_matrix(report, plots_dir)
    plot_stage4_entropy(report, plots_dir)
    plot_stage5_precursor(report, plots_dir)

    print(f"✅ Generated 9 publication-quality diagnostic charts in {plots_dir}")

if __name__ == '__main__':
    main()
