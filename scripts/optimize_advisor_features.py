#!/usr/bin/env python3
"""
Generate an advisor roster and metric ownership from recorded paper tape.

Optuna searches the enabled advisor roles and the evidence owned by each of
their semantic classes. Its objective is the executable quoted movement left
between a causal observation and the extremum of the canonical price episode
shown by Hindsight. The search sees only the leading chronological partition;
the trailing partition is reported once after selection.

The resulting configuration is an experiment for prospective paper trading,
not a claim of future profit. It refuses to overwrite its input unless
--promote is explicit.
"""

import argparse
import copy
import json
import math
import os
import sqlite3
import subprocess
import sys
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import optuna
import pandas as pd
MINIMUM_VARIANCE_CLASS_COUNT = 2

BULLISH_CLASSES = {
    "Building", "BuyersBreakingThrough", "Sustaining", "BroadLift",
    "Extending", "Replenishing", "LiquiditySweep", "WallBuilding",
    "LeverageSqueeze", "DiscountExpanding", "LocalLeader", "IsolatedMove",
}
BEARISH_CLASSES = {
    "OrderlyPullback", "FollowerMove", "SellersAbsorbing", "BuyersAbsorbing",
    "Reversing", "Exhausting", "GivingBack", "Depleting", "PremiumExpanding",
    "SellersBreakingThrough", "StructuralBreakdown", "VacuumForming",
    "LiquidationsCascading",
}
MOVE_PROJECTION = {
    "Building": {"explosive_pump": 1.5, "steady_trend": 1.0},
    "BuyersBreakingThrough": {"explosive_pump": 1.5, "steady_trend": 1.0},
    "Sustaining": {"steady_trend": 1.2},
    "BroadLift": {"steady_trend": 1.2},
    "Extending": {"steady_trend": 1.2},
    "Replenishing": {"steady_trend": 1.2},
    "LiquiditySweep": {"explosive_pump": 1.0, "weak_drift": 0.5},
    "WallBuilding": {"explosive_pump": 1.0, "weak_drift": 0.5},
    "LeverageSqueeze": {"explosive_pump": 1.0, "weak_drift": 0.5},
    "DiscountExpanding": {"explosive_pump": 1.0, "weak_drift": 0.5},
    "Stalling": {"stagnant": 1.5},
    "Balanced": {"stagnant": 1.5},
    "Consolidating": {"stagnant": 1.5},
    "NeutralBasis": {"stagnant": 1.5},
    "Unresolved": {"stagnant": 1.5},
    "OrderlyPullback": {"structural_pullback": 1.0, "stagnant": 0.5},
    "FollowerMove": {"structural_pullback": 1.0, "stagnant": 0.5},
    "SellersAbsorbing": {"stagnant": 1.0, "weak_bleed": 0.5},
    "BuyersAbsorbing": {"stagnant": 1.0, "weak_bleed": 0.5},
    "Reversing": {"structural_pullback": 1.0, "flash_dump": 0.8},
    "Exhausting": {"structural_pullback": 1.0, "flash_dump": 0.8},
    "GivingBack": {"structural_pullback": 1.0, "flash_dump": 0.8},
    "Depleting": {"structural_pullback": 1.0, "flash_dump": 0.8},
    "PremiumExpanding": {"structural_pullback": 1.0, "flash_dump": 0.8},
    "SellersBreakingThrough": {"flash_dump": 1.5, "structural_pullback": 1.0},
    "StructuralBreakdown": {"flash_dump": 1.5, "structural_pullback": 1.0},
    "VacuumForming": {"flash_dump": 1.5, "structural_pullback": 1.0},
    "LiquidationsCascading": {"flash_dump": 1.5, "structural_pullback": 1.0},
    "LocalLeader": {"steady_trend": 1.0, "explosive_pump": 0.5},
    "IsolatedMove": {"weak_drift": 1.0, "stagnant": 0.5},
}
VETOES = {
    (("momentum", "Building"), ("auction", "SellersAbsorbing")): "stagnant",
    (("momentum", "Building"), ("liquidity", "VacuumForming")): "flash_dump",
    (("profit_run", "Exhausting"), ("liquidity", "Depleting")): "flash_dump",
    (("pullback", "StructuralBreakdown"), ("auction", "SellersBreakingThrough")): "structural_pullback",
}
SYNERGIES = {
    (("pullback", "LiquiditySweep"), ("liquidity", "WallBuilding")): "explosive_pump",
    (("auction", "BuyersBreakingThrough"), ("basis", "LeverageSqueeze")): "explosive_pump",
    (("momentum", "Sustaining"), ("participation", "BroadLift")): "steady_trend",
    (("pullback", "OrderlyPullback"), ("basis", "DiscountExpanding")): "explosive_pump",
}
SOURCE_GROUPS = {
    "auction": "BookDepth",
    "basis": "Derivatives",
    "liquidity": "Liquidity",
    "momentum": "Hawkes",
    "participation": "OrderFlow",
    "profit_run": "TrendDynamics",
    "pullback": "TrendDynamics",
}


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
        if metadata.get("normative_status") in {
            "DEPRECATED",
            "MIGRATE_AND_REMOVE",
        }:
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


def unreferenced_metrics(path: str) -> set[str]:
    """Load producer identities with no declared runtime consumer."""
    if not os.path.isfile(path):
        raise ValueError(f"metric lineage not found: {path}")

    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)

    return {
        str(producer["id"])
        for producer in payload.get("producers", [])
        if producer.get("dead") and producer.get("id")
    }


def load_jsonl(path: str) -> List[Dict[str, Any]]:
    """Read one exported tape without changing its record order."""
    if not os.path.isfile(path):
        raise ValueError(f"JSONL input not found: {path}")

    records = []

    with open(path, "r", encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, start=1):
            stripped = line.strip()

            if not stripped:
                continue

            try:
                records.append(json.loads(stripped))
            except json.JSONDecodeError as err:
                raise ValueError(
                    f"invalid JSONL record at {path}:{line_number}: {err}"
                ) from err

    return records

def causal_screening_zscores(
    feature_values: pd.DataFrame,
    metadata: pd.DataFrame,
) -> pd.DataFrame:
    """Causally standardize every eligible coordinate for train-only screening."""
    result = pd.DataFrame(
        np.nan,
        index=feature_values.index,
        columns=feature_values.columns,
        dtype=float,
    )
    states: Dict[Tuple[str, str, str], Tuple[float, float, float]] = {}
    order = sorted(
        feature_values.index,
        key=lambda index: (
            str(metadata.at[index, "run"]),
            int(metadata.at[index, "issued_at"]),
            int(metadata.at[index, "sequence"]),
            str(metadata.at[index, "symbol"]),
        ),
    )

    for index in order:
        stream = (
            str(metadata.at[index, "run"]),
            str(metadata.at[index, "symbol"]),
        )

        for metric in feature_values.columns:
            value = feature_values.at[index, metric]

            if not np.isfinite(value):
                continue

            state_key = (stream[0], stream[1], metric)
            count, mean, m2 = states.get(state_key, (0.0, 0.0, 0.0))
            log_value = math.log(value) if value > 0 else 0.0

            if count == 0:
                score = 0.0
            else:
                dispersion = math.sqrt(m2 / (count - 1.0)) if count >= 2 else 0.0
                divergence = log_value - mean

                if dispersion <= 0:
                    dispersion = abs(divergence)

                score = divergence / dispersion if dispersion > 0 else 0.0

            result.at[index, metric] = score
            updated_count = count + 1.0
            delta = log_value - mean
            updated_mean = mean + delta / updated_count
            updated_m2 = m2 + delta * (log_value - updated_mean)
            states[state_key] = (updated_count, updated_mean, updated_m2)

    return result


def signed_correlation(values: np.ndarray, labels: np.ndarray) -> Optional[float]:
    """Return point-biserial correlation on complete observations."""
    complete = np.isfinite(values) & np.isfinite(labels)
    observed = values[complete]
    outcomes = labels[complete]

    if len(observed) < 2 or len(np.unique(outcomes)) < 2:
        return None

    if np.std(observed) == 0:
        return None

    correlation = float(np.corrcoef(observed, outcomes)[0, 1])

    if not np.isfinite(correlation):
        return None

    return correlation


def write_json(path: str, payload: Dict[str, Any]) -> None:
    """Write one JSON artifact atomically."""
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    temporary = path + ".tmp"

    with open(temporary, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, indent=2)
        handle.write("\n")

    os.replace(temporary, path)


def capture_runs(database_path: str, requested: Optional[str]) -> List[str]:
    """Resolve the tape partitions in causal run-start order."""
    if requested and requested != "all":
        return [requested]

    with sqlite3.connect(f"file:{database_path}?mode=ro", uri=True) as database:
        rows = database.execute(
            "SELECT id FROM runs ORDER BY started_at ASC"
        ).fetchall()

    runs = [str(row[0]) for row in rows if row[0]]

    if not runs:
        raise ValueError("Hindsight database contains no captured runs")

    return runs


def load_opportunity_tape(
    database_path: Optional[str],
    jsonl_path: Optional[str],
    requested_run: Optional[str],
    training_clock: str,
) -> List[Dict[str, Any]]:
    """Load retained observations plus canonical Hindsight price episodes."""
    if jsonl_path:
        return load_jsonl(jsonl_path)

    if not database_path or not os.path.isfile(database_path):
        raise ValueError(f"Hindsight database not found: {database_path}")

    records: List[Dict[str, Any]] = []

    for run_id in capture_runs(database_path, requested_run):
        command = [
            "go", "run", "-buildvcs=false", "./cmd/hindsight_export",
            database_path, "-run", run_id, "-metrics",
            "-training-clock", training_clock, "-opportunities",
        ]
        result = subprocess.run(command, capture_output=True, text=True, check=False)

        if result.returncode != 0:
            raise ValueError(
                f"hindsight_export failed for run {run_id}:\n{result.stderr.strip()}"
            )

        for line in result.stdout.splitlines():
            if line.strip():
                records.append(json.loads(line))

    return records


def episode_extremum(episode: Dict[str, Any]) -> Tuple[int, float]:
    """Read the endpoint coordinate and midpoint declared by Hindsight."""
    role = "peak" if episode.get("kind") == "upward_excursion" else "trough"

    for reference in episode.get("references", []):
        if reference.get("role") == role and reference.get("hasValue"):
            capture = reference.get("capture") or {}
            return int(capture.get("sequence", 0)), float(reference["value"])

    raise ValueError(f"episode {episode.get('id', '')} has no {role} reference")


def build_opportunity_dataset(
    records: Sequence[Dict[str, Any]],
    candidates: Sequence[str],
) -> Tuple[pd.DataFrame, pd.DataFrame]:
    """Align causal metric observations with Hindsight price legs."""
    episodes: Dict[Tuple[str, str], List[Dict[str, Any]]] = {}
    run_order: Dict[str, int] = {}

    for record in records:
        record_run = str(record.get("run", ""))

        if record_run not in run_order:
            run_order[record_run] = len(run_order)

        if record.get("kind") != "episode":
            continue

        episode = record.get("episode") or {}
        if episode.get("kind") not in {"upward_excursion", "downward_excursion"}:
            continue

        episodes.setdefault(
            (str(record.get("run", "")), str(episode.get("symbol", ""))), []
        ).append(record)

    rows: List[List[float]] = []
    metadata: List[Dict[str, Any]] = []

    for record in records:
        if record.get("kind") != "observation" or not record.get("hasQuote"):
            continue

        ask = float(record.get("ask", 0))
        bid = float(record.get("bid", 0))

        if ask <= 0 or bid <= 0 or ask < bid:
            continue

        run_id = str(record.get("run", ""))
        symbol = str(record.get("symbol", ""))
        sequence = int(record.get("sequence", 0))
        containing = [
            candidate for candidate in episodes.get((run_id, symbol), [])
            if int(candidate["episode"].get("fromSequence", 0)) <= sequence
            <= int(candidate["episode"].get("toSequence", 0))
            and candidate.get("hasExtremumBid")
        ]
        containing.sort(key=lambda candidate: (
            int(candidate["episode"].get("fromSequence", 0)) != sequence,
            int(candidate["episode"].get("toSequence", 0)) -
            int(candidate["episode"].get("fromSequence", 0)),
        ))
        episode_id = ""
        episode_kind = ""
        outcome_sequence = sequence
        gross_return = bid / ask - 1.0

        if containing:
            selected = containing[0]
            episode = selected["episode"]
            episode_id = str(episode.get("id", ""))
            episode_kind = str(episode.get("kind", ""))
            outcome_sequence, _ = episode_extremum(episode)
            exit_bid = float(selected["extremumBid"])
            gross_return = exit_bid / ask - 1.0

        metrics = record.get("metrics") or {}
        rows.append([
            float(metrics[key]) if key in metrics else np.nan
            for key in candidates
        ])
        metadata.append({
            "run": run_id,
            "run_order": run_order[run_id],
            "symbol": symbol,
            "sequence": sequence,
            "issued_at": int(record.get("observedAt", 0)) or sequence,
            "outcome_at": outcome_sequence,
            "coordinate": int(record.get("coordinate", 0)),
            "episode_id": episode_id,
            "episode_kind": episode_kind,
            "gross_return": gross_return,
            "spread_return": bid / ask - 1.0,
        })

    if not rows:
        raise ValueError("tape has no quoted retained Advisor observations")

    values = pd.DataFrame(rows, columns=list(candidates), dtype=float)
    details = pd.DataFrame(metadata)
    order = details.sort_values(
        ["run_order", "sequence", "symbol"], kind="stable"
    ).index

    return (
        values.loc[order].reset_index(drop=True),
        details.loc[order].reset_index(drop=True),
    )


def opportunity_split(
    metadata: pd.DataFrame,
    validation_fraction: float,
) -> Tuple[np.ndarray, np.ndarray]:
    """Reserve whole trailing episodes and purge legs crossing the boundary."""
    episodes: Dict[Tuple[str, str], List[int]] = {}

    for index in metadata.index[metadata["episode_id"] != ""]:
        key = (
            str(metadata.at[index, "run"]),
            str(metadata.at[index, "episode_id"]),
        )
        episodes.setdefault(key, []).append(int(index))

    if len(episodes) < 2:
        raise ValueError("tape has fewer than two quoted price episodes")

    ordered = sorted(episodes, key=lambda key: (
        int(metadata.at[episodes[key][0], "run_order"]),
        min(int(metadata.at[index, "sequence"]) for index in episodes[key]),
        key,
    ))
    split_at = max(1, min(
        int(len(ordered) * (1.0 - validation_fraction)),
        len(ordered) - 1,
    ))
    training_episodes = set(ordered[:split_at])
    validation_episodes = set(ordered[split_at:])
    validation_episode_rows = [
        index for key in validation_episodes for index in episodes[key]
    ]
    boundary_index = min(
        validation_episode_rows,
        key=lambda index: (
            int(metadata.at[index, "run_order"]),
            int(metadata.at[index, "sequence"]),
        ),
    )
    boundary = (
        int(metadata.at[boundary_index, "run_order"]),
        int(metadata.at[boundary_index, "sequence"]),
    )
    training: List[int] = []
    validation: List[int] = []

    for index in metadata.index:
        position = (
            int(metadata.at[index, "run_order"]),
            int(metadata.at[index, "sequence"]),
        )
        episode_id = str(metadata.at[index, "episode_id"])
        episode_key = (str(metadata.at[index, "run"]), episode_id)

        if position < boundary:
            if not episode_id:
                training.append(int(index))
                continue

            if episode_key in training_episodes and (
                int(metadata.at[index, "run_order"]),
                int(metadata.at[index, "outcome_at"]),
            ) < boundary:
                training.append(int(index))

            continue

        if not episode_id or episode_key in validation_episodes:
            validation.append(int(index))

    training_array = np.asarray(training, dtype=int)
    validation_array = np.asarray(validation, dtype=int)

    if len(training_array) == 0 or len(validation_array) == 0:
        raise ValueError("chronological purge left no train or validation observations")

    return training_array, validation_array


def class_target(class_name: str, returns: np.ndarray) -> np.ndarray:
    """Orient screening toward the move semantics the class already owns."""
    if class_name in BULLISH_CLASSES:
        return returns

    if class_name in BEARISH_CLASSES:
        return -returns

    return -np.abs(returns)


def jointly_comparable(
    zscores: pd.DataFrame,
    target: np.ndarray,
    training: np.ndarray,
    keys: Sequence[str],
) -> bool:
    """Report whether one complete evidence group remains measurable."""
    observations = zscores.loc[training, list(keys)].to_numpy(dtype=float)
    outcomes = target[training]
    complete = np.all(np.isfinite(observations), axis=1) & np.isfinite(outcomes)

    if np.sum(complete) < 2:
        return False

    composite = np.mean(observations[complete], axis=1)

    return signed_correlation(composite, outcomes[complete]) is not None


def candidate_rankings(
    config: Dict[str, Any],
    candidates: Sequence[str],
    zscores: pd.DataFrame,
    metadata: pd.DataFrame,
    training: np.ndarray,
) -> Dict[Tuple[str, str], List[str]]:
    """Rank every aligned metric by relevance and incremental independence."""
    returns = metadata["gross_return"].to_numpy(dtype=float)
    rankings: Dict[Tuple[str, str], List[str]] = {}
    correlations = zscores.loc[training, list(candidates)].corr().abs()

    for advisor_name, advisor_config in config.get("advisors", {}).items():
        for feature in advisor_config.get("features", []):
            target = class_target(str(feature["class"]), returns)
            relevance: Dict[str, float] = {}

            for key in candidates:
                correlation = signed_correlation(
                    zscores[key].to_numpy(dtype=float)[training],
                    target[training],
                )

                if correlation is not None and correlation > 0:
                    relevance[key] = correlation

            ordered: List[str] = []
            remaining = set(relevance)
            redundancy = {key: 0.0 for key in remaining}

            while remaining:
                ranked = sorted(
                    ((
                        relevance[key] - redundancy[key],
                        relevance[key],
                        key,
                    ) for key in remaining),
                    key=lambda item: (-item[0], -item[1], item[2]),
                )
                selected_key = ranked[0][2]
                ordered.append(selected_key)
                remaining.remove(selected_key)

                incomparable = []

                for key in remaining:
                    dependency = float(correlations.at[key, selected_key])

                    if not np.isfinite(dependency):
                        incomparable.append(key)
                        continue

                    redundancy[key] = max(redundancy[key], dependency)

                remaining.difference_update(incomparable)

            selected: List[str] = []

            for key in ordered:
                proposed = selected + [key]

                if jointly_comparable(zscores, target, training, proposed):
                    selected.append(key)

            if not selected:
                raise ValueError(
                    f"{advisor_name}/{feature['class']} has no observable candidates"
                )

            rankings[(advisor_name, str(feature["class"]))] = selected

    return rankings


def sample_candidate(
    trial: optuna.Trial,
    original: Dict[str, Any],
    rankings: Dict[Tuple[str, str], List[str]],
) -> Dict[str, Any]:
    """Ask Optuna for one complete council roster and ownership assignment."""
    candidate = copy.deepcopy(original)
    enabled: Dict[str, bool] = {}

    for advisor_name in sorted(candidate.get("advisors", {})):
        enabled[advisor_name] = trial.suggest_categorical(
            f"enabled/{advisor_name}", [True, False]
        )

    sources = {
        SOURCE_GROUPS.get(name, name)
        for name, active in enabled.items() if active
    }

    if sum(enabled.values()) < 3 or len(sources) < 3:
        raise optuna.TrialPruned()

    for advisor_name, advisor_config in candidate["advisors"].items():
        advisor_config["enabled"] = enabled[advisor_name]

        if not enabled[advisor_name]:
            continue

        for feature in advisor_config.get("features", []):
            class_name = str(feature["class"])
            ranking = rankings[(advisor_name, class_name)]
            count = trial.suggest_int(
                f"count/{advisor_name}/{class_name}", 1, len(ranking)
            )
            feature["keys"] = ranking[:count]

    return candidate


def classifier_readings(
    values: pd.DataFrame,
    metadata: pd.DataFrame,
    features: Sequence[Dict[str, Any]],
) -> List[Optional[Dict[str, Any]]]:
    """Run vector.Classifier's causal z-score, mean-logit, and softmax contract."""
    groups = [(str(feature["class"]), list(feature["keys"])) for feature in features]
    readings: List[Optional[Dict[str, Any]]] = [None] * len(values)
    moments: Dict[Tuple[str, str, str], Tuple[float, float, float]] = {}
    maturities: Dict[Tuple[str, str, str], float] = {}
    all_keys = sorted({key for _, keys in groups for key in keys})

    for index in range(len(values)):
        run_id = str(metadata.at[index, "run"])
        symbol = str(metadata.at[index, "symbol"])
        ready = [
            (label, keys) for label, keys in groups
            if all(np.isfinite(values.at[index, key]) for key in keys)
        ]

        if len(ready) < MINIMUM_VARIANCE_CLASS_COUNT:
            continue

        standardized: Dict[str, float] = {}

        for key in {key for _, keys in ready for key in keys}:
            state_key = (run_id, symbol, key)
            count, mean, m2 = moments.get(state_key, (0.0, 0.0, 0.0))
            value = float(values.at[index, key])
            log_value = math.log(value) if value > 0 else 0.0
            divergence = log_value - mean
            dispersion = math.sqrt(m2 / (count - 1.0)) if count >= 2 else 0.0

            if dispersion <= 0:
                dispersion = abs(divergence)

            standardized[key] = divergence / dispersion if dispersion > 0 else 0.0
            updated = count + 1.0
            delta = log_value - mean
            updated_mean = mean + delta / updated
            moments[state_key] = (
                updated,
                updated_mean,
                m2 + delta * (log_value - updated_mean),
            )
            maturities[state_key] = count / (count + 1.0)

        logits = np.asarray([
            float(np.mean([standardized[key] for key in keys]))
            for _, keys in ready
        ])
        weights = np.exp(logits - np.max(logits))
        probabilities = weights / np.sum(weights)
        positive = probabilities[probabilities > 0]
        entropy = -float(np.sum(positive * np.log(positive)))
        sharpness = 1.0 - entropy / math.log(len(probabilities))

        if sharpness <= 0:
            continue

        winner = int(np.argmax(probabilities))

        if np.sum(np.abs(probabilities - probabilities[winner]) <= 1e-9) > 1:
            continue

        maturity = min(
            maturities.get((run_id, symbol, key), 0.0) for key in all_keys
        )
        readings[index] = {
            "top": ready[winner][0],
            "maturity": maturity,
            "probabilities": {
                label: float(probabilities[position])
                for position, (label, _) in enumerate(ready)
            },
        }

    return readings


def council_signals(
    config: Dict[str, Any],
    values: pd.DataFrame,
    metadata: pd.DataFrame,
) -> np.ndarray:
    """Apply the War Room projection and opportunity admission contract."""
    readings = {
        advisor_name: classifier_readings(
            values, metadata, advisor_config.get("features", [])
        )
        for advisor_name, advisor_config in config.get("advisors", {}).items()
        if advisor_config.get("enabled", True)
    }
    horizons = {
        (advisor_name, str(feature["class"])): int(feature.get("within", 0))
        for advisor_name, advisor_config in config.get("advisors", {}).items()
        if advisor_config.get("enabled", True)
        for feature in advisor_config.get("features", [])
    }
    residents: Dict[Tuple[str, str, str], Tuple[int, Dict[str, Any]]] = {}
    signals = np.zeros(len(values), dtype=bool)

    for index in range(len(values)):
        run_id = str(metadata.at[index, "run"])
        symbol = str(metadata.at[index, "symbol"])
        coordinate = int(metadata.at[index, "coordinate"])

        for name, advisor_readings in readings.items():
            reading = advisor_readings[index]

            if reading is None:
                continue

            horizon = horizons.get((name, str(reading["top"])), 0)

            if horizon <= 0:
                raise ValueError(
                    f"{name}/{reading['top']} has no valid prediction horizon"
                )

            residents[(run_id, symbol, name)] = (
                coordinate + horizon, reading
            )

        active = {}

        for name in readings:
            resident = residents.get((run_id, symbol, name))

            if resident is None:
                continue

            lease_until, reading = resident

            if coordinate > lease_until:
                del residents[(run_id, symbol, name)]
                continue

            active[name] = reading

        if len(active) < 3:
            continue

        if len({SOURCE_GROUPS.get(name, name) for name in active}) < 3:
            continue

        top = {(name, str(reading["top"])) for name, reading in active.items()}
        vetoed = any(
            first in top and second in top for first, second in VETOES
        )
        group_counts: Dict[str, int] = {}

        for name in active:
            group = SOURCE_GROUPS.get(name, name)
            group_counts[group] = group_counts.get(group, 0) + 1

        mass = {
            "explosive_pump": 0.05, "steady_trend": 0.15,
            "weak_drift": 0.15, "stagnant": 0.30, "weak_bleed": 0.15,
            "structural_pullback": 0.15, "flash_dump": 0.05,
        }

        for name, reading in active.items():
            group_count = group_counts[SOURCE_GROUPS.get(name, name)]
            discount = 1.0 / math.sqrt(group_count)
            factor = max(float(reading["maturity"]), 0.4) * discount

            for class_name, probability in reading["probabilities"].items():
                projection = MOVE_PROJECTION.get(class_name)

                if projection is None:
                    raise ValueError(f"War Room has no projection for {class_name}")

                for move, weight in projection.items():
                    mass[move] += probability * factor * weight

        for (first, second), move in {**VETOES, **SYNERGIES}.items():
            if first not in top or second not in top:
                continue

            first_reading = active[first[0]]
            second_reading = active[second[0]]
            joint = (
                first_reading["probabilities"][first[1]] *
                second_reading["probabilities"][second[1]] *
                max(float(first_reading["maturity"]), 0.4) *
                max(float(second_reading["maturity"]), 0.4)
            )

            if (first, second) in VETOES:
                suppression = max(1.0 - joint, 0.05)
                mass["explosive_pump"] *= suppression
                mass["steady_trend"] *= suppression
                mass[move] += joint * 3.0
                continue

            mass[move] += joint * 4.0

        total = sum(max(value, 0.01) for value in mass.values())
        probabilities = {
            move: max(value, 0.01) / total for move, value in mass.items()
        }
        upward = (
            probabilities["explosive_pump"] + probabilities["steady_trend"] +
            0.5 * probabilities["weak_drift"]
        )
        downward = (
            probabilities["flash_dump"] +
            probabilities["structural_pullback"] +
            0.5 * probabilities["weak_bleed"]
        )
        signals[index] = not vetoed and upward > downward and upward >= 0.5

    return signals


def capitalization_score(
    signals: np.ndarray,
    metadata: pd.DataFrame,
    indices: np.ndarray,
) -> Dict[str, Any]:
    """Score one entry per Hindsight price leg at the first council signal."""
    episodes: Dict[Tuple[str, str], List[int]] = {}

    for index in indices:
        episode_id = str(metadata.at[index, "episode_id"])

        if episode_id:
            episodes.setdefault(
                (str(metadata.at[index, "run"]), episode_id), []
            ).append(int(index))

    realized: List[float] = []
    upward_seen = 0
    downward_seen = 0
    episode_rows = {index for rows in episodes.values() for index in rows}
    false_entries = [
        int(index) for index in indices
        if signals[index] and int(index) not in episode_rows
    ]

    realized.extend(
        float(metadata.at[index, "spread_return"])
        for index in false_entries
    )

    for rows in episodes.values():
        rows.sort(key=lambda index: int(metadata.at[index, "sequence"]))
        entered = next((index for index in rows if signals[index]), None)

        if entered is None:
            continue

        outcome = float(metadata.at[entered, "gross_return"])
        realized.append(outcome)

        if metadata.at[entered, "episode_kind"] == "upward_excursion":
            upward_seen += 1
        else:
            downward_seen += 1

    score = sum(math.log1p(value) for value in realized if value > -1)
    denominator = max(len(episodes), 1)

    return {
        "objective": score / denominator,
        "episodes": len(episodes),
        "entries": len(realized),
        "opportunityEntries": upward_seen + downward_seen,
        "falseEntries": len(false_entries),
        "upwardEntries": upward_seen,
        "downwardEntries": downward_seen,
        "grossReturnSum": sum(realized),
    }


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate advisors against canonical Hindsight opportunities"
    )
    parser.add_argument("--db", default=os.path.expanduser("~/.symm/data/events.sqlite"))
    parser.add_argument("--jsonl", help="Pre-exported observation and episode JSONL")
    parser.add_argument("--run", default="all", help="Captured run identity or all")
    parser.add_argument("--config", default="config/advisors.json")
    parser.add_argument("--out", default="runs/advisors.candidate.json")
    parser.add_argument("--report", help="Defaults to <out>.report.json")
    parser.add_argument("--metric-map", default="signal/metric_map.json")
    parser.add_argument("--lineage", default="frontend/public/metric-lineage.json")
    parser.add_argument("--validation-fraction", type=float, default=0.25)
    parser.add_argument("--trials", type=int, default=200)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument(
        "--promote",
        action="store_true",
        help="also write the winner to the active paper advisor config",
    )
    args = parser.parse_args()

    try:
        report_path = args.report or args.out + ".report.json"

        if not 0 < args.validation_fraction < 1:
            raise ValueError("--validation-fraction must be in (0, 1)")

        if args.trials < 1:
            raise ValueError("--trials must be positive")

        if not os.path.isfile(args.config):
            raise ValueError(f"advisor config not found: {args.config}")

        active_config = os.path.abspath(os.path.expanduser(
            "~/.symm/data/advisors.json"
        ))
        protected_outputs = {os.path.abspath(args.config), active_config}

        if os.path.abspath(args.out) in protected_outputs and not args.promote:
            raise ValueError(
                "refusing to overwrite an input or active paper config "
                "without --promote"
            )

        if os.path.abspath(report_path) in {
            os.path.abspath(args.config), os.path.abspath(args.out)
        }:
            raise ValueError("audit report requires its own path")

        with open(args.config, "r", encoding="utf-8") as handle:
            original = json.load(handle)

        advisors = original.get("advisors", {})
        clocks = {str(config.get("clock", "")) for config in advisors.values()}

        if "" in clocks or len(clocks) != 1:
            raise ValueError("all candidate advisors must share one declared clock")

        catalog = load_metric_catalog(args.metric_map)
        candidates = eligible_catalog_candidates(
            {"clock": next(iter(clocks)), "features": [
                feature for advisor_config in advisors.values()
                for feature in advisor_config.get("features", [])
            ]},
            catalog,
        )
        records = load_opportunity_tape(
            args.db, args.jsonl, args.run, next(iter(clocks))
        )
        values, metadata = build_opportunity_dataset(records, candidates)
        training, validation = opportunity_split(
            metadata, args.validation_fraction
        )
        screening = causal_screening_zscores(values, metadata)
        rankings = candidate_rankings(
            original, candidates, screening, metadata, training,
        )
        optuna.logging.set_verbosity(optuna.logging.WARNING)
        sampler = optuna.samplers.TPESampler(seed=args.seed)
        study = optuna.create_study(direction="maximize", sampler=sampler)
        candidates_by_trial: Dict[int, Dict[str, Any]] = {}

        def objective(trial: optuna.Trial) -> float:
            candidate = sample_candidate(trial, original, rankings)
            candidates_by_trial[trial.number] = candidate
            signals = council_signals(candidate, values, metadata)
            return float(capitalization_score(
                signals, metadata, training
            )["objective"])

        study.optimize(objective, n_trials=args.trials)
        best = candidates_by_trial[study.best_trial.number]
        signals = council_signals(best, values, metadata)
        train_result = capitalization_score(signals, metadata, training)
        validation_result = capitalization_score(signals, metadata, validation)
        unreferenced = unreferenced_metrics(args.lineage)
        owned = {
            key
            for advisor_config in best["advisors"].values()
            if advisor_config.get("enabled", True)
            for feature in advisor_config.get("features", [])
            for key in feature.get("keys", [])
        }
        report = {
            "source": {
                "database": args.db if not args.jsonl else None,
                "jsonl": args.jsonl,
                "requestedRun": args.run,
                "runs": sorted(set(metadata["run"])),
                "observations": len(metadata),
            },
            "policy": {
                "target": "quoted movement remaining in canonical Hindsight price episodes",
                "selection": "Optuna joint roster and metric ownership search",
                "validation": "trailing chronological partition never shown to Optuna",
                "execution": "entry at retained ask; exit at episode extremum bid when captured",
                "fees": "not recorded on the tape and therefore not fabricated",
                "nonOpportunitySignal": "charged the immediately executable quoted spread",
                "arena": "candidate prediction falsification is not replayed",
                "stoploss": "not replayed; generated Perspectives flow through the real StopLoss in prospective paper trading",
                "candidateUniverse": len(candidates),
                "trials": args.trials,
                "ranking": "all aligned metrics ordered by causal relevance and incremental non-redundancy",
                "cardinality": "Optuna chooses up to the data-supported jointly comparable prefix",
                "seed": args.seed,
            },
            "dataSupportedKeys": {
                f"{advisor_name}/{class_name}": len(ranking)
                for (advisor_name, class_name), ranking in rankings.items()
            },
            "enabledAdvisors": sorted(
                name for name, config in best["advisors"].items()
                if config.get("enabled", True)
            ),
            "ownedMetrics": len(owned),
            "ownedUnreferencedMetrics": len(owned & unreferenced),
            "training": train_result,
            "validation": validation_result,
            "bestTrial": study.best_trial.number,
            "bestParameters": study.best_trial.params,
        }
        write_json(args.out, best)
        write_json(report_path, report)

        if args.promote and os.path.abspath(args.out) != active_config:
            write_json(active_config, best)

        print(f"wrote candidate config: {args.out}")
        print(f"wrote audit report: {report_path}")

        if args.promote:
            print(f"promoted paper config: {active_config}")

        print("enabled advisors: " + ", ".join(report["enabledAdvisors"]))
        print(f"training objective: {train_result['objective']:.12f}")
        print(f"validation objective: {validation_result['objective']:.12f}")
        print(
            "owned metrics: "
            f"{report['ownedMetrics']} "
            f"({report['ownedUnreferencedMetrics']} lineage-unreferenced)"
        )

        return 0
    except (
        OSError, ValueError, sqlite3.Error, subprocess.SubprocessError,
    ) as err:
        print(f"Error: {err}", file=sys.stderr)

        return 1


if __name__ == "__main__":
    sys.exit(main())
