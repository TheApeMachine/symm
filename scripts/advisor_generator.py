#!/usr/bin/env python3
"""
Optuna Advisor Generator for SYMM.

Optimizes Advisor feature selection, prediction contracts, and horizons
across dual objectives:
  1. Prediction Validity (Falsifiable claim survival AUC).
  2. Stoploss Utility (Exhaustion peak timing, sweep rebound, & cascade persistence).

Self-contained script: contains all data-loading, causal Welford standardization,
and purged chronological split routines.
"""

import argparse
import bisect
import copy
import json
import math
import os
import shutil
import subprocess
import sys
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import optuna
import pandas as pd
from scipy.stats import mannwhitneyu

# Ensure repository root is on sys.path if needed
REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if REPO_ROOT not in sys.path:
    sys.path.insert(0, REPO_ROOT)

CHANCE_AUC = 0.5
MINIMUM_VARIANCE_CLASS_COUNT = 2
_RECORDS_CACHE: Dict[Tuple[str, str, str, str, int], List[Dict[str, Any]]] = {}


# =========================================================================
# Data Loading & Catalog Helpers
# =========================================================================

def load_metric_catalog(path: str) -> Dict[str, Dict[str, Any]]:
    """Load the metric schema by its source-qualified identity."""
    if not os.path.isfile(path):
        raise ValueError(f"metric catalog not found: {path}")

    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)

    catalog: Dict[str, Dict[str, Any]] = {}
    for entry in payload.get("metrics", []):
        identity = entry.get("identity")
        if identity:
            catalog[identity] = entry

    return catalog


def eligible_catalog_candidates(
    advisor_config: Dict[str, Any],
    catalog: Dict[str, Dict[str, Any]],
) -> List[str]:
    """Return metrics whose catalog contract permits decision evidence."""
    configured = set()
    for feature in advisor_config.get("features", []):
        configured.update(feature.get("keys", []))

    for key in sorted(configured):
        if key not in catalog:
            raise ValueError(f"configured metric is absent from metric catalog: {key}")

    clock = advisor_config.get("clock", "")
    usable = []

    for key, metadata in sorted(catalog.items()):
        if metadata.get("normative_status") in {"DEPRECATED", "MIGRATE_AND_REMOVE"}:
            continue
        if metadata.get("semantic_role") == "ESTIMABILITY":
            continue
        if metadata.get("metric_class") == "support_or_inference":
            continue
        if key == clock:
            continue
        usable.append(key)

    if not usable:
        raise ValueError("metric catalog has no eligible candidate metrics")

    return usable


def load_raw_hindsight_records(
    database_path: Optional[str],
    jsonl_path: Optional[str],
    run_id: Optional[str],
    training_clock: str,
    limit: int,
) -> List[Dict[str, Any]]:
    """Load observations and Perspectives from JSONL or hindsight_export."""
    cache_key = (
        database_path or "",
        jsonl_path or "",
        run_id or "",
        training_clock,
        limit,
    )

    if cache_key in _RECORDS_CACHE:
        return _RECORDS_CACHE[cache_key]

    if jsonl_path:
        if not os.path.isfile(jsonl_path):
            raise ValueError(f"JSONL input not found: {jsonl_path}")

        records = []
        with open(jsonl_path, "r", encoding="utf-8") as handle:
            for line_number, line in enumerate(handle, start=1):
                stripped = line.strip()
                if not stripped:
                    continue
                try:
                    records.append(json.loads(stripped))
                except json.JSONDecodeError as err:
                    raise ValueError(
                        f"invalid JSONL record at {jsonl_path}:{line_number}: {err}"
                    ) from err

        _RECORDS_CACHE[cache_key] = records
        return records

    if not database_path or not os.path.isfile(database_path):
        raise ValueError(f"Hindsight database not found: {database_path}")

    binary = None
    for candidate_bin in ("./bin/hindsight_export", "hindsight_export"):
        resolved = shutil.which(candidate_bin)
        if resolved:
            binary = resolved
            break

    if binary:
        command = [
            binary,
            database_path,
            "-perspectives",
            "-metrics",
            "-training-clock",
            training_clock,
            "-limit",
            str(limit),
        ]
    else:
        command = [
            "go",
            "run",
            "-buildvcs=false",
            "./cmd/hindsight_export",
            database_path,
            "-perspectives",
            "-metrics",
            "-training-clock",
            training_clock,
            "-limit",
            str(limit),
        ]

    if run_id:
        command.extend(["-run", run_id])

    result = subprocess.run(
        command,
        capture_output=True,
        text=True,
        timeout=300,
        check=False,
    )

    if result.returncode != 0:
        raise ValueError("hindsight_export failed:\n" + result.stderr.strip())

    records = []
    for line in result.stdout.splitlines():
        stripped = line.strip()
        if stripped:
            records.append(json.loads(stripped))

    _RECORDS_CACHE[cache_key] = records
    return records


def issued_records(
    records: Sequence[Dict[str, Any]],
    advisor_name: str,
) -> List[Dict[str, Any]]:
    """Return unique issued rows for one advisor in event-time order."""
    unique: Dict[Tuple[str, str, int], Dict[str, Any]] = {}

    for record in records:
        if record.get("advisor") != advisor_name or record.get("lifecycle") != "issued":
            continue

        required = (
            "run",
            "symbol",
            "sequence",
            "claimSequence",
            "issuedAt",
            "leaseFrom",
            "leaseUntil",
        )
        if any(field not in record for field in required):
            raise ValueError("Perspective export lacks event-time fields")

        key = (
            str(record["run"]),
            str(record["symbol"]),
            int(record["claimSequence"]),
        )
        existing = unique.get(key)
        if existing is None or int(record["sequence"]) < int(existing["sequence"]):
            unique[key] = record

    return sorted(
        unique.values(),
        key=lambda record: (
            str(record["run"]),
            int(record["issuedAt"]),
            int(record["sequence"]),
            str(record["symbol"]),
        ),
    )


def build_lifecycle_dataset(
    records: Sequence[Dict[str, Any]],
    advisor_name: str,
    advisor_config: Dict[str, Any],
    candidates: Sequence[str],
) -> Tuple[pd.DataFrame, pd.DataFrame, Dict[str, np.ndarray]]:
    """Label retained observations from Arena's persisted lifecycle."""
    features = advisor_config.get("features", [])
    
    # 1. Filter strictly for this advisor's perspectives and shared observations
    scoped_records = [
        r for r in records
        if r.get("kind") == "observation" or r.get("advisor") == advisor_name
    ]

    issued = issued_records(scoped_records, advisor_name)
    survived: Dict[Tuple[str, str, str, int], Dict[str, Any]] = {}
    coordinate_times: Dict[Tuple[str, str, str], Dict[int, int]] = {}
    issued_by_coordinate: Dict[Tuple[str, str, str, int], Dict[str, Any]] = {}
    observations = [r for r in scoped_records if r.get("kind") == "observation"]

    if not observations:
        raise ValueError(
            f"input lacks retained training observations for advisor '{advisor_name}'; export with -training-clock"
        )

    # 2. Key survived records by (run, symbol, advisor, claimSequence)
    for record in scoped_records:
        if record.get("advisor") == advisor_name and record.get("lifecycle") == "survived":
            claim = (
                str(record["run"]),
                str(record["symbol"]),
                str(record["advisor"]),
                int(record.get("claimSequence", 0)),
            )
            survived[claim] = record

    for perspective in issued:
        stream = (
            str(perspective["run"]),
            str(perspective["symbol"]),
            str(perspective.get("clock", "")),
        )
        coordinate = int(perspective["leaseFrom"])
        key = stream + (coordinate,)
        # Deduplicate multiple emissions at the same clock coordinate gracefully
        issued_by_coordinate[key] = perspective

    for observation in observations:
        stream = (
            str(observation["run"]),
            str(observation["symbol"]),
            str(observation.get("clock", "")),
        )
        coordinate = int(observation["coordinate"])
        observed_at = int(observation["observedAt"])
        coordinate_times.setdefault(stream, {})[coordinate] = observed_at

    ordered_coordinates = {
        stream: sorted(times) for stream, times in coordinate_times.items()
    }

    ordered_observations = sorted(
        observations,
        key=lambda record: (
            str(record["run"]),
            int(record["observedAt"]),
            int(record["sequence"]),
            str(record["symbol"]),
        ),
    )

    candidate_to_idx = {k: i for i, k in enumerate(candidates)}
    n_obs = len(ordered_observations)
    n_metrics = len(candidates)
    values_matrix = np.full((n_obs, n_metrics), np.nan, dtype=float)

    metadata_rows: List[Dict[str, Any]] = []
    labels: Dict[str, List[float]] = {feature["class"]: [] for feature in features}

    for obs_idx, observation in enumerate(ordered_observations):
        metrics = observation.get("metrics") or {}
        for k, v in metrics.items():
            col_idx = candidate_to_idx.get(k)
            if col_idx is not None:
                try:
                    values_matrix[obs_idx, col_idx] = float(v)
                except (ValueError, TypeError):
                    values_matrix[obs_idx, col_idx] = np.nan

        stream = (
            str(observation["run"]),
            str(observation["symbol"]),
            str(observation.get("clock", "")),
        )
        coordinate_key = stream + (int(observation["coordinate"]),)
        perspective = issued_by_coordinate.get(coordinate_key)

        outcome: Optional[float] = None
        outcome_at = 0

        if perspective is not None:
            claim = (
                str(perspective["run"]),
                str(perspective["symbol"]),
                str(perspective["advisor"]),
                int(perspective["claimSequence"]),
            )
            resolution = survived.get(claim)

            if resolution is not None:
                # Same advisor and claim: confirm matching class
                if resolution.get("class") == perspective.get("class"):
                    outcome = 1.0
                    outcome_at = int(resolution.get("resolvedAt", 0))
            else:
                boundary = int(perspective["leaseUntil"])
                coordinates = ordered_coordinates.get(stream, [])
                closing_index = bisect.bisect_left(coordinates, boundary)

                if closing_index < len(coordinates):
                    closing_coordinate = coordinates[closing_index]
                    closing_time = coordinate_times[stream].get(closing_coordinate, 0)
                    outcome = 0.0
                    outcome_at = closing_time

        metadata_rows.append({
            "run": str(observation["run"]),
            "symbol": str(observation["symbol"]),
            "issued_at": int(observation["observedAt"]),
            "outcome_at": outcome_at,
            "sequence": int(observation["sequence"]),
            "bid": float(observation.get("bid", 0.0)),
            "ask": float(observation.get("ask", 0.0)),
        })

        for feature in features:
            assigned_outcome = (
                outcome
                if perspective is not None
                and feature["class"] == perspective.get("class")
                and outcome is not None
                and outcome_at > 0
                else np.nan
            )
            labels[feature["class"]].append(assigned_outcome)

    return (
        pd.DataFrame(values_matrix, columns=list(candidates), dtype=float),
        pd.DataFrame(metadata_rows),
        {
            class_name: np.asarray(values, dtype=float)
            for class_name, values in labels.items()
        },
    )

# =========================================================================
# Vectorized Causal Standardization & Chronological Splitting
# =========================================================================

def causal_zscores(
    feature_values: pd.DataFrame,
    metadata: pd.DataFrame,
    groups: Sequence[Sequence[str]],
) -> pd.DataFrame:
    """Vectorized causal standardization mirroring vector.Logits."""
    values = feature_values.to_numpy(dtype=float)
    n_rows, n_cols = values.shape
    result = np.full((n_rows, n_cols), np.nan, dtype=float)

    col_map = {col: i for i, col in enumerate(feature_values.columns)}
    group_indices = [
        [col_map[k] for k in group if k in col_map] for group in groups
    ]

    order = np.lexsort((
        metadata["symbol"].astype(str).to_numpy(),
        metadata["sequence"].astype(int).to_numpy(),
        metadata["issued_at"].astype(int).to_numpy(),
        metadata["run"].astype(str).to_numpy(),
    ))

    stream_keys = list(zip(metadata["run"], metadata["symbol"]))
    unique_streams = {k: i for i, k in enumerate(sorted(set(stream_keys)))}
    stream_ids = np.array([unique_streams[k] for k in stream_keys], dtype=int)

    n_streams = len(unique_streams)
    counts = np.zeros((n_streams, n_cols), dtype=float)
    means = np.zeros((n_streams, n_cols), dtype=float)
    m2s = np.zeros((n_streams, n_cols), dtype=float)

    for idx in order:
        s_id = stream_ids[idx]
        row_vals = values[idx]

        ready = [
            g for g in group_indices
            if g and np.all(np.isfinite(row_vals[g]))
        ]

        if len(ready) < MINIMUM_VARIANCE_CLASS_COUNT:
            continue

        scored_cols = np.unique(np.concatenate(ready))
        v = row_vals[scored_cols]
        c = counts[s_id, scored_cols]
        m = means[s_id, scored_cols]
        m2 = m2s[s_id, scored_cols]

        dispersion = np.zeros_like(c)
        valid_var = c >= 2
        dispersion[valid_var] = np.sqrt(m2[valid_var] / (c[valid_var] - 1.0))

        divergence = v - m
        score = np.zeros_like(v)
        has_variance = (c > 0) & (dispersion > 1e-8)
        score[has_variance] = divergence[has_variance] / dispersion[has_variance]

        result[idx, scored_cols] = score

        updated_c = c + 1.0
        delta = v - m
        updated_m = m + delta / updated_c
        updated_m2 = m2 + delta * (v - updated_m)

        counts[s_id, scored_cols] = updated_c
        means[s_id, scored_cols] = updated_m
        m2s[s_id, scored_cols] = updated_m2

    return pd.DataFrame(result, index=feature_values.index, columns=feature_values.columns)


def compute_chronological_boundary(
    metadata: pd.DataFrame,
    validation_fraction: float,
) -> int:
    """Find a synchronized event-time boundary separating training from validation."""
    issued_times = metadata["issued_at"].dropna().astype(int).to_numpy()
    if len(issued_times) == 0:
        return 0
    sorted_times = np.sort(issued_times)
    split_idx = int(len(sorted_times) * (1.0 - validation_fraction))
    split_idx = max(1, min(split_idx, len(sorted_times) - 1))
    return int(sorted_times[split_idx])


def purged_chronological_split(
    metadata: pd.DataFrame,
    labels: np.ndarray,
    validation_start_time: int,
) -> Tuple[np.ndarray, np.ndarray]:
    """Split at fixed cutoff and purge training observations whose outcomes overlap validation."""
    valid_indices = np.flatnonzero(np.isfinite(labels))
    if len(valid_indices) < 2:
        return np.array([], dtype=int), np.array([], dtype=int)

    issued_times = metadata["issued_at"].to_numpy()
    outcome_times = metadata["outcome_at"].to_numpy()

    validation = np.asarray([
        idx for idx in valid_indices
        if issued_times[idx] >= validation_start_time
    ], dtype=int)

    training = np.asarray([
        idx for idx in valid_indices
        if issued_times[idx] < validation_start_time
        and 0 < outcome_times[idx] < validation_start_time
    ], dtype=int)

    return training, validation


# =========================================================================
# Multi-Objective Evaluation: Prediction Survival & Stoploss Utility
# =========================================================================

def evaluate_advisor_stoploss_utility(
    zscores: pd.DataFrame,
    metadata: pd.DataFrame,
    trial_features: List[Dict[str, Any]],
    val_indices: np.ndarray,
) -> float:
    """
    Measures how effectively the advisor's signals anticipate forward price movement:
    1. Exhaustion (Stalling/GivingBack): should anticipate forward drawdowns.
    2. Rebound (Sweep/WallBuilding): should anticipate forward rebounds.
    3. Reflexive Momentum (Building/Sustaining): should anticipate persistent forward trend.
    """
    if len(val_indices) < 20:
        return 0.5

    # 1. Bearish / Exhaustion / Giveback (should precede local peaks & drawdowns)
    exhaust_classes = {
        "Stalling", "Reversing", "Exhausting", "GivingBack", "VacuumForming",
        "SellersAbsorbing", "SellersBreakingThrough", "IsolatedMove", "DiscountExpanding"
    }

    # 2. Bullish / Sweep Rebound / Support (should precede upward bounce at lows)
    rebound_classes = {
        "LiquiditySweep", "WallBuilding", "Replenishing", "BuyersAbsorbing",
        "OrderlyPullback"
    }

    # 3. Reflexive Continuation / Momentum (should precede sustained upward runs)
    cascade_classes = {
        "Building", "Sustaining", "LeverageSqueeze", "PremiumExpanding",
        "BuyersBreakingThrough", "BroadLift", "Extending"
    }

    class_names = {f["class"] for f in trial_features}
    tracked_exhaust = exhaust_classes & class_names
    tracked_rebound = rebound_classes & class_names
    tracked_cascade = cascade_classes & class_names

    if not (tracked_exhaust or tracked_rebound or tracked_cascade):
        return 0.5

    bids = metadata["bid"].to_numpy()
    n_rows = len(metadata)

    # 5-bar forward return proxy
    forward_returns = np.full(n_rows, np.nan, dtype=float)
    valid_bids = bids > 0
    for i in range(n_rows - 5):
        if valid_bids[i] and valid_bids[i + 5]:
            forward_returns[i] = (bids[i + 5] - bids[i]) / bids[i]

    val_fwd_ret = forward_returns[val_indices]
    finite_ret_mask = np.isfinite(val_fwd_ret)
    if np.count_nonzero(finite_ret_mask) < 15:
        return 0.5

    scores_by_class = {}
    for feat in trial_features:
        keys = [k for k in feat["keys"] if k in zscores.columns]
        if keys:
            mat = zscores.loc[val_indices, keys].to_numpy()
            valid_rows = np.all(np.isfinite(mat), axis=1)
            mean_score = np.full(len(val_indices), np.nan, dtype=float)
            mean_score[valid_rows] = np.mean(mat[valid_rows], axis=1)
            scores_by_class[feat["class"]] = mean_score

    utility_components = []

    # 1. Exhaustion check: high score -> negative forward return
    for c_name in tracked_exhaust:
        scores = scores_by_class.get(c_name)
        if scores is None:
            continue
        valid = np.isfinite(scores) & finite_ret_mask
        if np.count_nonzero(valid) >= 10:
            corr = np.corrcoef(scores[valid], val_fwd_ret[valid])[0, 1]
            if np.isfinite(corr):
                # We want negative correlation with forward return
                utility_components.append(0.5 - 0.5 * corr)

    # 2. Rebound check: high score -> positive forward return
    for c_name in tracked_rebound:
        scores = scores_by_class.get(c_name)
        if scores is None:
            continue
        valid = np.isfinite(scores) & finite_ret_mask
        if np.count_nonzero(valid) >= 10:
            corr = np.corrcoef(scores[valid], val_fwd_ret[valid])[0, 1]
            if np.isfinite(corr):
                utility_components.append(0.5 + 0.5 * corr)

    # 3. Cascade/Momentum check: high score -> positive forward return
    for c_name in tracked_cascade:
        scores = scores_by_class.get(c_name)
        if scores is None:
            continue
        valid = np.isfinite(scores) & finite_ret_mask
        if np.count_nonzero(valid) >= 10:
            corr = np.corrcoef(scores[valid], val_fwd_ret[valid])[0, 1]
            if np.isfinite(corr):
                utility_components.append(0.5 + 0.5 * corr)

    return float(np.mean(utility_components)) if utility_components else 0.5


def evaluate_advisor_trial(
    trial: optuna.Trial,
    advisor_name: str,
    base_advisor_config: Dict[str, Any],
    candidate_pool: List[str],
    raw_values: pd.DataFrame,
    metadata: pd.DataFrame,
    labels: Dict[str, np.ndarray],
    validation_cutoff: int,
) -> Tuple[float, float]:
    """Evaluates an Optuna trial returning (Mean Survival AUC, Stoploss Utility)."""
    trial_features = []
    MOVE_PAIRS = [
        "INCREASE:DECREASE",
        "DECREASE:INCREASE",
        "EXPAND:DISSOLVE",
        "DISSOLVE:EXPAND",
        "STAGNATE:EXPAND",
        "EXPAND:STAGNATE",
    ]

    for base_feat in base_advisor_config.get("features", []):
        class_name = base_feat["class"]

        # 1. Number of features per class (3 to 6)
        n_features = trial.suggest_int(f"{class_name}_num_keys", 3, 6)
        selected_keys = []
        for i in range(n_features):
            k = trial.suggest_categorical(f"{class_name}_key_{i}", candidate_pool)
            if k not in selected_keys:
                selected_keys.append(k)

        # Ensure at least 2 distinct keys are present
        if len(selected_keys) < 2:
            return 0.0, 0.0

        # 2. Lease horizon (within)
        within = trial.suggest_int(f"{class_name}_within", 1, 3)

        # 3. Static Falsifiable prediction contract
        predictions = []
        if base_feat.get("predictions"):
            pred_metric = trial.suggest_categorical(f"{class_name}_pred_metric", candidate_pool)
            move_pair = trial.suggest_categorical(f"{class_name}_move_pair", MOVE_PAIRS)
            support_move, contradict_move = move_pair.split(":")
            predictions.append({
                "metric": pred_metric,
                "support": support_move,
                "contradict": contradict_move,
            })

        trial_features.append({
            "class": class_name,
            "within": within,
            "keys": selected_keys,
            "predictions": predictions,
        })

    groups = [f["keys"] for f in trial_features]
    zscores = causal_zscores(raw_values, metadata, groups)

    class_aucs = []
    all_val_indices = []

    for feat in trial_features:
        class_name = feat["class"]
        class_labels = labels[class_name]

        training, validation = purged_chronological_split(
            metadata,
            class_labels,
            validation_cutoff,
        )
        if len(validation) > 0:
            all_val_indices.extend(validation)

        keys = feat["keys"]
        if not keys or len(validation) == 0:
            continue

        mat = zscores.loc[validation, keys].to_numpy()
        complete = np.all(np.isfinite(mat), axis=1)
        if np.count_nonzero(complete) < 5:
            continue

        scores = np.mean(mat[complete], axis=1)
        outcomes = class_labels[validation][complete]

        pos = scores[outcomes == 1]
        neg = scores[outcomes == 0]

        if len(pos) >= 2 and len(neg) >= 2:
            test = mannwhitneyu(pos, neg, alternative="greater", method="auto")
            auc = float(test.statistic / (len(pos) * len(neg)))
            class_aucs.append(auc)

    mean_auc = float(np.mean(class_aucs)) if class_aucs else 0.5
    stoploss_utility = evaluate_advisor_stoploss_utility(
        zscores, metadata, trial_features, np.unique(all_val_indices)
    )

    return mean_auc, stoploss_utility


# =========================================================================
# Main Optimization Runner
# =========================================================================

def optimize_single_advisor(
    advisor_name: str,
    advisor_cfg: Dict[str, Any],
    catalog: Dict[str, Dict[str, Any]],
    raw_records: List[Dict[str, Any]],
    trials: int,
    validation_fraction: float,
) -> List[Dict[str, Any]]:
    """Runs Optuna for a single advisor and returns the optimized feature configurations."""
    # Pre-filter records for this advisor and observations
    scoped_records = [
        r for r in raw_records
        if r.get("kind") == "observation" or r.get("advisor") == advisor_name
    ]

    candidates = eligible_catalog_candidates(advisor_cfg, catalog)
    values, metadata, labels = build_lifecycle_dataset(
        scoped_records, advisor_name, advisor_cfg, candidates
    )
    val_cutoff = compute_chronological_boundary(metadata, validation_fraction)

    print(f"[{advisor_name}] Starting Multi-Objective Optuna Study ({trials} trials)...")
    optuna.logging.set_verbosity(optuna.logging.WARNING)
    sampler = optuna.samplers.NSGAIISampler(seed=42)
    study = optuna.create_study(
        directions=["maximize", "maximize"],  # [Survival AUC, Stoploss Utility]
        sampler=sampler,
    )

    def objective(trial: optuna.Trial):
        return evaluate_advisor_trial(
            trial,
            advisor_name,
            advisor_cfg,
            candidates,
            values,
            metadata,
            labels,
            val_cutoff,
        )

    study.optimize(objective, n_trials=trials, show_progress_bar=True)

    best_trials = study.best_trials
    chosen_trial = max(best_trials, key=lambda t: t.values[0] + t.values[1])
    print(
        f"[{advisor_name}] Selected Trial {chosen_trial.number}: "
        f"Survival AUC = {chosen_trial.values[0]:.4f}, "
        f"Stoploss Utility = {chosen_trial.values[1]:.4f}"
    )

    optimized_features = []
    for base_feat in advisor_cfg.get("features", []):
        c_name = base_feat["class"]
        n_keys = chosen_trial.params[f"{c_name}_num_keys"]
        keys = []
        for i in range(n_keys):
            k = chosen_trial.params[f"{c_name}_key_{i}"]
            if k not in keys:
                keys.append(k)

        within = chosen_trial.params[f"{c_name}_within"]
        predictions = []
        if base_feat.get("predictions"):
            move_pair = chosen_trial.params[f"{c_name}_move_pair"]
            support_move, contradict_move = move_pair.split(":")
            predictions.append({
                "metric": chosen_trial.params[f"{c_name}_pred_metric"],
                "support": support_move,
                "contradict": contradict_move,
            })

        optimized_features.append({
            "class": c_name,
            "within": within,
            "keys": keys,
            "predictions": predictions,
        })

    return optimized_features


def main():
    parser = argparse.ArgumentParser(description="Optimize SYMM Advisors with Optuna")
    parser.add_argument(
        "--db",
        default=os.path.expanduser("~/.symm/data/events.sqlite"),
        help="Hindsight SQLite database",
    )
    parser.add_argument(
        "--jsonl",
        help="Pre-exported Perspective JSONL",
    )
    parser.add_argument(
        "--config",
        default="config/advisors.json",
        help="Advisor JSON configuration",
    )
    parser.add_argument(
        "--metric-map",
        default="signal/metric_map.json",
        help="Metric schema catalog",
    )
    parser.add_argument(
        "--advisor",
        default="all",
        help="Target advisor name (e.g. momentum, pullback) or 'all'",
    )
    parser.add_argument(
        "--trials",
        type=int,
        default=50,
        help="Number of Optuna trials per advisor",
    )
    parser.add_argument(
        "--out",
        default="config/advisors.json",
        help="Output path for optimized advisor config",
    )
    parser.add_argument(
        "--validation-fraction",
        type=float,
        default=0.25,
        help="Holdout validation fraction",
    )
    args = parser.parse_args()

    # Load configuration
    config_path = args.config
    if not os.path.isabs(config_path) and not os.path.exists(config_path):
        config_path = os.path.join(REPO_ROOT, config_path)

    metric_map_path = args.metric_map
    if not os.path.isabs(metric_map_path) and not os.path.exists(metric_map_path):
        metric_map_path = os.path.join(REPO_ROOT, metric_map_path)

    with open(config_path, "r", encoding="utf-8") as handle:
        full_config = json.load(handle)

    advisors = full_config.get("advisors", {})
    target_advisors = list(advisors.keys()) if args.advisor == "all" else [args.advisor]

    for name in target_advisors:
        if name not in advisors:
            raise ValueError(f"Advisor {name} not found in {config_path}")

    catalog = load_metric_catalog(metric_map_path)

    # Determine clocks
    clocks = {str(advisors[name].get("clock", "")) for name in target_advisors}
    if "" in clocks:
        raise ValueError("Selected advisor has no market clock declared")
    if len(clocks) > 1:
        raise ValueError(f"Selected advisors declare divergent market clocks: {clocks}")

    training_clock = next(iter(clocks))
    print(f"Loading Hindsight records for clock '{training_clock}'...")
    raw_records = load_raw_hindsight_records(args.db, args.jsonl, None, training_clock, 0)
    print(f"Loaded {len(raw_records)} records.")

    optimized_config = copy.deepcopy(full_config)

    for name in target_advisors:
        opt_features = optimize_single_advisor(
            name,
            advisors[name],
            catalog,
            raw_records,
            args.trials,
            args.validation_fraction,
        )
        optimized_config["advisors"][name]["features"] = opt_features

    out_path = args.out
    if not os.path.isabs(out_path) and not os.path.exists(os.path.dirname(out_path)):
        out_path = os.path.join(REPO_ROOT, out_path)

    os.makedirs(os.path.dirname(os.path.abspath(out_path)), exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as handle:
        json.dump(optimized_config, handle, indent=2)
        handle.write("\n")

    print(f"\nWrote optimized advisor configuration to {out_path}")


if __name__ == "__main__":
    main()