#!/usr/bin/env python3
"""
SYMM Audit Visualization Engine
Produces intuitive, publication-quality diagnostic charts with embedded plain-English
explanations underneath titles (never obscuring data) and complete, untruncated tick labels.
"""

import sys
import os
import json
import argparse
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

def add_header(fig, ax, main_title, meaning, good_criteria, higher_lower, pad=58):
    """
    Renders the main title and an explanation banner card positioned directly
    underneath the title, above the plot axes, ensuring data is NEVER obscured.
    """
    ax.set_title(main_title, fontsize=12.5, fontweight='bold', pad=pad, color='#f0f6fc')
    
    explanation = (
        f"[WHAT THIS MEANS]: {meaning}\n"
        f"[WHAT GOOD LOOKS LIKE]: {good_criteria}\n"
        f"[SCALE]: {higher_lower}"
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
        f"[WHAT GOOD LOOKS LIKE]: {good_criteria}\n"
        f"[SCALE]: {higher_lower}"
    )
    fig.text(
        0.5, 0.915,
        explanation,
        ha='center', va='top',
        fontsize=8.0,
        color='#adbac7',
        bbox=dict(boxstyle='round,pad=0.5', facecolor='#1c2128', edgecolor='#373e47', alpha=0.95)
    )
    fig.subplots_adjust(top=0.79)

def plot_stage0_contract(report, out_dir):
    contract = report.get('contract', {})
    breaches = contract.get('breaches', [])
    total_checked = contract.get('total_metrics_checked', 0)
    
    fig, ax = plt.subplots(figsize=(14, 8))
    
    if not breaches:
        ax.text(0.5, 0.5, f"[OK] All {total_checked} Metrics Comply with Declared Contracts\nZero values emitted outside stated mathematical domains.",
                ha='center', va='center', fontsize=12, color='#2ea043', transform=ax.transAxes)
    else:
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
    
    add_header(
        fig, ax,
        f'Stage 0: Metric Contract Integrity ({len(breaches)} Breaching Metrics Found)',
        'Audits whether sensors strictly adhere to their declared mathematical domains without clamping.',
        '0 contract breaches. All correlation estimators strictly bounded to [-1, 1] or [0, 1].',
        '0 BREACHES IS REQUIRED (Breaches indicate ungrounded finite-sample Hayashi-Yoshida estimators).'
    )
    
    fig.tight_layout()
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
    
    add_figure_header(
        fig,
        'Stage 1: Sensor Vitality & Universe Dimension Mapping',
        'Compares raw producer output series (peer-qualified) against the canonical collapsed grid universe.',
        f'Raw: {raw_healthy}/{raw_total} healthy. Canonical Grid: {canon_healthy}/{canon_total} healthy.',
        'DESCRIPTIVE ONLY (Coverage and constancy are observations, not a health cutoff).'
    )
    
    fig.savefig(os.path.join(out_dir, 'stage1_metric_vitality.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage1_redundancy(report, out_dir):
    redundant = report.get('vitality', {}).get('redundant_pairs', [])
    pairs = [f"{p['metric_a']} <-> {p['metric_b']}" for p in redundant[:18]]
    corrs = [p['correlation'] for p in redundant[:18]]

    fig_height = max(6.0, len(pairs) * 0.42 + 2.5) if pairs else 6.0
    fig, ax = plt.subplots(figsize=(14, fig_height))

    if not redundant:
        ax.text(0.5, 0.5, "[OK] Zero Redundant Canonical Clones Found (|r| >= 0.95)\nEvery healthy cell represents an independent dimension.",
                ha='center', va='center', fontsize=12, color='#2ea043', transform=ax.transAxes)
    else:
        ax.barh(range(len(pairs)), corrs, color='#58a6ff', height=0.55)
        ax.set_yticks(range(len(pairs)))
        ax.set_yticklabels(pairs, fontsize=8)
        ax.set_xlabel('Pearson Correlation Coefficient (r)', fontsize=10, color='#adbac7')
        ax.set_xlim(0.9, 1.02)
        ax.invert_yaxis()
        ax.tick_params(axis='y', labelsize=8)

    add_header(
        fig, ax,
        f'Stage 1: Canonical Metric Redundancy (|r| >= 0.95, {len(redundant)} Pairs Found)',
        'Pairs of canonical cells tracking each other so closely they share representation subspace.',
        'Redundancy between baselines, z-scores, and raw signals confirms the need for grid dimensionality reduction.',
        'VALIDATES GRID COMPRESSION (Finding collinearity proves the grid is needed to merge related channels).'
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

    add_header(
        fig, ax,
        'Stage 2: Pair Sympathy vs. Shuffled Null on Deformations',
        'Tests whether real scale-free deformations show significant correlation beyond a permutation null.',
        'Real distribution shows significant tails outside the null envelope (KS > 0.10, Sep > 10%).',
        'SEPARATION RATIO & KS DISTANCE (Higher indicates genuine sympathetic market structure).'
    )

    fig.tight_layout()
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

    add_header(
        fig, ax,
        'Stage 2: Sympathy Orientation (Lockstep vs. Inverse Opposition)',
        'Classifies pairwise deformation movements into positive co-movement vs stable inverse opposition.',
        'Both modes represent real market structure. Production affinity uses |r| to recognize both as affinity.',
        'DIVERSITY OF ORIENTATION (Confirms both positive and negative coupling exist in the asset ecosystem).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage2_orientation_balance.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage3_grid_stability(report, out_dir):
    stab = report.get('grid_stability', {})
    grid_a = stab.get('grid_a', {}).get('region_sizes', {})
    grid_b = stab.get('grid_b', {}).get('region_sizes', {})
    ari = stab.get('adjusted_rand_idx', 0)
    overlap = stab.get('overlap_fraction', 0) * 100
    shared_u = stab.get('shared_universe', 0)

    all_regions = sorted(list(set(list(grid_a.keys()) + list(grid_b.keys()))), key=lambda r: int(r.replace('R', '')) if r.replace('R', '').isdigit() else 999)

    sizes_a = [grid_a.get(r, 0) for r in all_regions]
    sizes_b = [grid_b.get(r, 0) for r in all_regions]

    x = np.arange(len(all_regions))
    width = 0.38

    fig, ax = plt.subplots(figsize=(13, 7))
    ax.bar(x - width/2, sizes_a, width, label='Early Period Grid', color='#58a6ff')
    ax.bar(x + width/2, sizes_b, width, label='Late Period Grid', color='#3fb950')

    ax.set_xticks(x)
    ax.set_xticklabels(all_regions, fontsize=8.5)
    ax.set_xlabel('Learned Spectral Region', fontsize=10, color='#adbac7')
    ax.set_ylabel('Number of Assigned Metric Cells', fontsize=10, color='#adbac7')
    ax.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=9)
    ax.grid(axis='y')

    add_header(
        fig, ax,
        f'Stage 3: Cross-Period Grid Stability (Adjusted Rand Index = {ari:.3f})',
        'Tests whether independent grids trained on disjoint chronological periods form reproducible partitions.',
        f'ARI = {ari:.3f} on {shared_u} shared cells ({overlap:.1f}% universe overlap). Region sizes (~5%) are structural.',
        'HIGHER ARI IS BETTER (1.0 = identical cluster memberships across disjoint time windows).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage3_region_partitioning.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_token_dynamics(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    freqs = dynamics.get('token_frequencies', {})
    total = max(1, dynamics.get('total_emissions', 0))

    sorted_tokens = sorted(freqs.keys(), key=lambda t: freqs[t], reverse=True)
    counts = [freqs[t] for t in sorted_tokens]
    shares = [c / total * 100 for c in counts]

    fig, ax = plt.subplots(figsize=(13, 7))
    bars = ax.bar(sorted_tokens, shares, color='#58a6ff', width=0.55, edgecolor='#22272e')

    for bar, pct in zip(bars, shares):
        ax.text(bar.get_x() + bar.get_width()/2, pct + 0.8, f"{pct:.1f}%", ha='center', va='bottom', fontsize=8, color='#adbac7')

    ax.axhline(80, color='#f85149', linestyle='--', label='Max Dominance Cap (80%)')
    ax.set_ylabel('Emission Frequency (%)', fontsize=10, color='#adbac7')
    ax.set_xlabel('Region Token', fontsize=10, color='#adbac7')
    ax.legend(loc='upper right', frameon=True, facecolor='#161b22', edgecolor='#30363d', fontsize=8.5)
    ax.grid(axis='y')

    add_header(
        fig, ax,
        f'Stage 4: Out-of-Sample Token Emissions ({total} Emissions across {len(freqs)} Regions)',
        'Frequency of region emissions when replaying unseen market tape through the frozen grid.',
        'Distributed activity across multiple active regions with no single region monopolizing (>80%).',
        'EVEN/MODERATE DOMINANCE IS BETTER (Confirms the state space actively differentiates regimes).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_token_dynamics.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_transition_matrix(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    transitions = dynamics.get('transitions', {})

    tokens = sorted(list(transitions.keys()), key=lambda r: int(r.replace('R', '')) if r.replace('R', '').isdigit() else 999)
    if not tokens:
        return

    n = len(tokens)
    matrix = np.zeros((n, n))

    for i, from_tok in enumerate(tokens):
        row_map = transitions.get(from_tok, {})
        row_total = sum(row_map.values())
        if row_total > 0:
            for j, to_tok in enumerate(tokens):
                matrix[i, j] = row_map.get(to_tok, 0) / row_total

    fig, ax = plt.subplots(figsize=(10, 8.5))
    im = ax.imshow(matrix, cmap='viridis', interpolation='nearest')

    ax.set_xticks(range(n))
    ax.set_yticks(range(n))
    ax.set_xticklabels(tokens, fontsize=8)
    ax.set_yticklabels(tokens, fontsize=8)
    ax.set_xlabel('Next Region Token (t+1)', fontsize=10, color='#adbac7')
    ax.set_ylabel('Current Region Token (t)', fontsize=10, color='#adbac7')

    cbar = fig.colorbar(im, ax=ax, fraction=0.046, pad=0.04)
    cbar.set_label('Transition Probability P(t+1 | t)', color='#adbac7', fontsize=9)

    add_header(
        fig, ax,
        'Stage 4: Out-of-Sample Region State Transition Matrix',
        'Empirical probability distribution of transitions between consecutive regional states.',
        'Structured pathways (bright diagonal/off-diagonal corridors) rather than diffuse uniform smear.',
        'DESCRIPTIVE TRANSITION GEOMETRY (Compare against the temporal null; do not grade by appearance).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_transition_matrix.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage4_entropy(report, out_dir):
    dynamics = report.get('token_dynamics', {})
    real_ent = dynamics.get('transition_entropy', 0)
    null_ent = dynamics.get('null_transition_entropy', 0)
    red_bits = dynamics.get('entropy_reduction_bits', 0)

    fig, ax = plt.subplots(figsize=(9, 6.5))
    bars = ax.bar(['Real Observed Transitions', 'Block-Shuffled Temporal Null'],
                   [real_ent, null_ent],
                   color=['#3fb950', '#8b949e'],
                   width=0.45, edgecolor='#22272e')

    for bar, val in zip(bars, [real_ent, null_ent]):
        ax.text(bar.get_x() + bar.get_width()/2, val + 0.05, f"{val:.3f} bits", ha='center', va='bottom', fontsize=10, color='#adbac7')

    ax.set_ylabel('Conditional Transition Entropy (Bits)', fontsize=10, color='#adbac7')
    ax.grid(axis='y')

    add_header(
        fig, ax,
        f'Stage 4: Out-of-Sample Transition Entropy (Reduction = {red_bits:.3f} Bits)',
        'Information entropy of the next-state token given the current state on held-out market tape.',
        'Real entropy lower than block-shuffled null (indicates non-trivial sequence structure beyond stickiness).',
        'LOWER REAL ENTROPY IS BETTER (Transition predictability indicates learnable tape structure).'
    )

    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, 'stage4_transition_entropy.png'), dpi=180, bbox_inches='tight')
    plt.close(fig)

def plot_stage5_precursor(report, out_dir):
    prec = report.get('precursor', {})
    det_found = prec.get('detections_found', 0)
    classes = prec.get('excursions_found', [])
    ign = prec.get('ignition_hypothesis', {})
    exh = prec.get('exhaustion_hypothesis', {})
    bg_tokens = prec.get('background_tokens', {})

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 7.5))

    # Panel 1: Hypothesis A -> B Ignition Precursor
    ign_jsd = ign.get('divergence_bits', 0)
    ign_null = ign.get('null_divergence_95', 0)
    ign_status = ign.get('status', 'INSUFFICIENT_DATA')
    ign_n = ign.get('event_token_count', 0)

    ign_color = '#58a6ff' if ign_status == 'MEASURED' else '#d29922'
    ax1.bar(['Ignition Divergence', 'Null-95 Bound'], [ign_jsd, ign_null], color=[ign_color, '#8b949e'], width=0.45)
    ax1.set_ylabel('Divergence vs Control (Bits)', fontsize=10, color='#adbac7')
    ax1.set_title(f'Hypothesis A->B (Ignition): {ign_status} (N={ign_n})', fontsize=11, fontweight='bold')
    ax1.grid(axis='y')

    # Panel 2: Hypothesis B -> C Exhaustion Precursor
    exh_jsd = exh.get('divergence_bits', 0)
    exh_null = exh.get('null_divergence_95', 0)
    exh_status = exh.get('status', 'INSUFFICIENT_DATA')
    exh_n = exh.get('event_token_count', 0)

    exh_color = '#58a6ff' if exh_status == 'MEASURED' else '#d29922'
    ax2.bar(['Exhaustion Divergence', 'Null-95 Bound'], [exh_jsd, exh_null], color=[exh_color, '#8b949e'], width=0.45)
    ax2.set_ylabel('Divergence vs Holding (Bits)', fontsize=10, color='#adbac7')
    ax2.set_title(f'Hypothesis B->C (Exhaustion): {exh_status} (N={exh_n})', fontsize=11, fontweight='bold')
    ax2.grid(axis='y')

    add_figure_header(
        fig,
        f'Stage 5: Precursor Informativeness (Detections: {det_found}, Classes: {", ".join(classes) if classes else "none"})',
        'Tests whether state distributions prior to ignition (A->B) and exhaustion (B->C) diverge from controls.',
        f'A->B status: {ign_status}. B->C status: {exh_status}. Background control: {len(bg_tokens)} tokens.',
        'EMPIRICAL COMPARISON ONLY (Absence of evidence is INSUFFICIENT_DATA; measured divergence is reported against its null).'
    )

    fig.savefig(os.path.join(out_dir, 'stage5_precursor_separation.png'), dpi=180, bbox_inches='tight')
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
    plot_stage3_grid_stability(report, plots_dir)
    plot_stage4_token_dynamics(report, plots_dir)
    plot_stage4_transition_matrix(report, plots_dir)
    plot_stage4_entropy(report, plots_dir)
    plot_stage5_precursor(report, plots_dir)

    print(f"✅ Generated 10 publication-quality diagnostic charts in {plots_dir}")

if __name__ == '__main__':
    main()
