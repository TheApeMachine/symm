import json
from pathlib import Path
import time
import unittest
import warnings

import numpy as np
import optuna
import pandas as pd

from optimize_advisor_features import (
    MOVE_PROJECTION,
    build_opportunity_dataset,
    candidate_rankings,
    causal_screening_zscores,
    capitalization_score,
    classifier_readings,
    council_signals,
    eligible_catalog_candidates,
    opportunity_split,
    sample_candidate,
)


def feature(class_name="Building"):
    return {
        "class": class_name,
        "within": 1,
        "keys": ["signal/precursor", "signal/noise"],
        "predictions": [{
            "metric": "signal/outcome",
            "support": "INCREASE",
            "contradict": "DECREASE",
        }],
    }

class OptimizeAdvisorFeaturesTest(unittest.TestCase):
    def test_eligible_catalog_candidates_include_unconfigured_measurements(self):
        catalog = {
            "signal/precursor": {
                "normative_status": "KEEP_NAMED_USE",
                "semantic_role": "FLOW_STATE",
                "metric_class": "direct_or_derived_measurement",
            },
            "signal/unreferenced": {
                "normative_status": "UNMAPPED_REVIEW",
                "semantic_role": "ACTIVITY_STATE",
                "metric_class": "temporal_dynamic_or_rate",
            },
            "signal/support": {
                "normative_status": "UNMAPPED_REVIEW",
                "semantic_role": "ESTIMABILITY",
                "metric_class": "support_or_inference",
            },
            "signal/clock": {
                "normative_status": "KEEP_NAMED_USE",
                "semantic_role": "ACTIVITY_STATE",
                "metric_class": "structural_metric",
            },
        }
        candidate_feature = feature()
        candidate_feature["keys"] = ["signal/precursor"]
        config = {
            "clock": "signal/clock",
            "features": [candidate_feature],
        }

        candidates = eligible_catalog_candidates(config, catalog)

        self.assertEqual(candidates, ["signal/precursor", "signal/unreferenced"])


    def test_causal_screening_zscores_advance_on_unlabelled_observations(self):
        values = pd.DataFrame({"signal/value": [1.0, 2.0, 4.0]})
        metadata = pd.DataFrame([
            {"run": "r", "symbol": "s", "issued_at": 1, "sequence": 1},
            {"run": "r", "symbol": "s", "issued_at": 2, "sequence": 2},
            {"run": "r", "symbol": "s", "issued_at": 3, "sequence": 3},
        ])

        scores = causal_screening_zscores(values, metadata)

        self.assertEqual(scores.iloc[0, 0], 0.0)
        self.assertEqual(scores.iloc[1, 0], 1.0)
        self.assertGreater(scores.iloc[2, 0], 0.0)

    def test_candidate_rankings_consider_every_observable_aligned_metric(self):
        config = {"advisors": {"momentum": {"features": [
            {"class": "Building"},
        ]}}}
        candidates = ["strong", "additional", "unavailable", "inverse"]
        scores = pd.DataFrame({
            "strong": [0.0, 1.0, 2.0, 3.0],
            "additional": [0.0, 0.5, 1.5, 3.0],
            "unavailable": [np.nan, np.nan, np.nan, 1.0],
            "inverse": [3.0, 2.0, 1.0, 0.0],
        })
        metadata = pd.DataFrame({"gross_return": [0.0, 1.0, 2.0, 3.0]})

        rankings = candidate_rankings(
            config, candidates, scores, metadata, np.arange(4),
        )

        self.assertEqual(
            set(rankings[("momentum", "Building")]),
            {"strong", "additional"},
        )

    def test_sample_candidate_lets_optuna_choose_full_supported_prefix(self):
        config = {"advisors": {
            "auction": {"features": [
                {"class": "BuyersBreakingThrough", "keys": ["old"]},
                {"class": "Balanced", "keys": ["old"]},
            ]},
            "momentum": {"features": [
                {"class": "Building", "keys": ["old"]},
                {"class": "Stalling", "keys": ["old"]},
            ]},
            "participation": {"features": [
                {"class": "BroadLift", "keys": ["old"]},
                {"class": "Unresolved", "keys": ["old"]},
            ]},
        }}
        rankings = {
            (advisor_name, feature["class"]): ["first", "second", "third"]
            for advisor_name, advisor_config in config["advisors"].items()
            for feature in advisor_config["features"]
        }
        parameters = {
            f"enabled/{advisor_name}": True
            for advisor_name in config["advisors"]
        }
        parameters.update({
            f"count/{advisor_name}/{feature['class']}": 3
            for advisor_name, advisor_config in config["advisors"].items()
            for feature in advisor_config["features"]
        })

        candidate = sample_candidate(
            optuna.trial.FixedTrial(parameters), config, rankings,
        )

        for advisor_config in candidate["advisors"].values():
            for candidate_feature in advisor_config["features"]:
                self.assertEqual(
                    candidate_feature["keys"], ["first", "second", "third"],
                )


    def test_build_opportunity_dataset_prices_entry_at_ask_and_exit_at_bid(self):
        records = [{
            "kind": "episode",
            "run": "run-1",
            "hasExtremumBid": True,
            "extremumBid": 120.0,
            "episode": {
                "id": "up-1",
                "symbol": "BTC/USD",
                "kind": "upward_excursion",
                "fromSequence": 1,
                "toSequence": 3,
                "references": [{
                    "role": "peak",
                    "hasValue": True,
                    "value": 121.0,
                    "capture": {"sequence": 3},
                }],
            },
        }, {
            "kind": "observation",
            "run": "run-1",
            "symbol": "BTC/USD",
            "sequence": 2,
            "observedAt": 2,
            "coordinate": 1,
            "hasQuote": True,
            "bid": 99.0,
            "ask": 100.0,
            "metrics": {"signal/value": 7.0},
        }]

        values, metadata = build_opportunity_dataset(records, ["signal/value"])

        self.assertEqual(values.iloc[0]["signal/value"], 7.0)
        self.assertEqual(metadata.iloc[0]["episode_id"], "up-1")
        self.assertAlmostEqual(metadata.iloc[0]["gross_return"], 0.2)

    def test_build_opportunity_dataset_does_not_invent_an_extremum_quote(self):
        records = [{
            "kind": "episode",
            "run": "run-1",
            "hasExtremumBid": False,
            "episode": {
                "id": "up-1",
                "symbol": "BTC/USD",
                "kind": "upward_excursion",
                "fromSequence": 1,
                "toSequence": 3,
                "references": [{
                    "role": "peak",
                    "hasValue": True,
                    "value": 121.0,
                    "capture": {"sequence": 3},
                }],
            },
        }, {
            "kind": "observation",
            "run": "run-1",
            "symbol": "BTC/USD",
            "sequence": 2,
            "coordinate": 1,
            "hasQuote": True,
            "bid": 99.0,
            "ask": 100.0,
            "metrics": {"signal/value": 7.0},
        }]

        _, metadata = build_opportunity_dataset(records, ["signal/value"])

        self.assertEqual(metadata.iloc[0]["episode_id"], "")
        self.assertAlmostEqual(metadata.iloc[0]["gross_return"], -0.01)

    def test_opportunity_split_keeps_episodes_whole_and_outside_rows_visible(self):
        metadata = pd.DataFrame([
            {"run": "r", "run_order": 0, "sequence": 1, "outcome_at": 2, "episode_id": "up"},
            {"run": "r", "run_order": 0, "sequence": 2, "outcome_at": 2, "episode_id": "up"},
            {"run": "r", "run_order": 0, "sequence": 3, "outcome_at": 3, "episode_id": ""},
            {"run": "r", "run_order": 0, "sequence": 4, "outcome_at": 5, "episode_id": "next"},
            {"run": "r", "run_order": 0, "sequence": 5, "outcome_at": 5, "episode_id": "next"},
            {"run": "r", "run_order": 0, "sequence": 6, "outcome_at": 6, "episode_id": ""},
        ])

        training, validation = opportunity_split(metadata, 0.5)

        self.assertEqual(training.tolist(), [0, 1, 2])
        self.assertEqual(validation.tolist(), [3, 4, 5])

    def test_classifier_readings_use_competing_causal_metric_groups(self):
        values = pd.DataFrame({
            "bull": [1.0, 4.0],
            "flat": [1.0, 1.0],
        })
        metadata = pd.DataFrame([
            {"run": "run-1", "symbol": "BTC/USD", "coordinate": 1},
            {"run": "run-1", "symbol": "BTC/USD", "coordinate": 2},
        ])
        readings = classifier_readings(values, metadata, [
            {"class": "Building", "keys": ["bull"]},
            {"class": "Stalling", "keys": ["flat"]},
        ])

        self.assertIsNone(readings[0])
        self.assertEqual(readings[1]["top"], "Building")

    def test_classifier_readings_keep_zero_softmax_mass_defined(self):
        values = pd.DataFrame({
            "bull": [1.0, 1.000000001, 1e300],
            "flat": [1.0, 1.0, 1.0],
        })
        metadata = pd.DataFrame([
            {"run": "run-1", "symbol": "BTC/USD"},
            {"run": "run-1", "symbol": "BTC/USD"},
            {"run": "run-1", "symbol": "BTC/USD"},
        ])

        with warnings.catch_warnings():
            warnings.simplefilter("error", RuntimeWarning)
            readings = classifier_readings(values, metadata, [
                {"class": "Building", "keys": ["bull"]},
                {"class": "Stalling", "keys": ["flat"]},
            ])

        self.assertEqual(readings[2]["top"], "Building")

    def test_council_signals_require_three_independent_bullish_advisors(self):
        values = pd.DataFrame({
            "bull": [1.0, 4.0],
            "flat": [1.0, 1.0],
        })
        metadata = pd.DataFrame([
            {"run": "run-1", "symbol": "BTC/USD", "coordinate": 1},
            {"run": "run-1", "symbol": "BTC/USD", "coordinate": 2},
        ])
        config = {"advisors": {
            "momentum": {"enabled": True, "features": [
                {"class": "Building", "within": 1, "keys": ["bull"]},
                {"class": "Stalling", "within": 1, "keys": ["flat"]},
            ]},
            "auction": {"enabled": True, "features": [
                {"class": "BuyersBreakingThrough", "within": 1, "keys": ["bull"]},
                {"class": "Balanced", "within": 1, "keys": ["flat"]},
            ]},
            "participation": {"enabled": True, "features": [
                {"class": "BroadLift", "within": 1, "keys": ["bull"]},
                {"class": "Unresolved", "within": 1, "keys": ["flat"]},
            ]},
        }}

        signals = council_signals(config, values, metadata)

        self.assertFalse(signals[0])
        self.assertTrue(signals[1])

    def test_capitalization_score_enters_each_episode_once(self):
        metadata = pd.DataFrame([
            {"run": "r", "episode_id": "up", "episode_kind": "upward_excursion", "sequence": 1, "gross_return": 0.2, "spread_return": -0.01},
            {"run": "r", "episode_id": "up", "episode_kind": "upward_excursion", "sequence": 2, "gross_return": 0.1, "spread_return": -0.01},
            {"run": "r", "episode_id": "down", "episode_kind": "downward_excursion", "sequence": 3, "gross_return": -0.1, "spread_return": -0.01},
            {"run": "r", "episode_id": "", "episode_kind": "", "sequence": 4, "gross_return": -0.02, "spread_return": -0.02},
        ])
        result = capitalization_score(
            np.asarray([True, True, False, True]), metadata, np.arange(4)
        )

        self.assertEqual(result["entries"], 2)
        self.assertEqual(result["falseEntries"], 1)
        self.assertEqual(result["upwardEntries"], 1)
        self.assertAlmostEqual(result["grossReturnSum"], 0.18)

    def test_war_room_projection_covers_every_configured_class(self):
        config_path = Path(__file__).resolve().parents[1] / "config/advisors.json"

        with config_path.open("r", encoding="utf-8") as handle:
            config = json.load(handle)

        classes = {
            str(feature["class"])
            for advisor in config["advisors"].values()
            for feature in advisor["features"]
        }

        self.assertEqual(classes - set(MOVE_PROJECTION), set())


def benchmark_opportunity_council(sample_count=3353):
    random = np.random.default_rng(9)
    values = pd.DataFrame({
        "bull": random.lognormal(size=sample_count),
        "flat": random.lognormal(size=sample_count),
    })
    metadata = pd.DataFrame({
        "run": ["run-1"] * sample_count,
        "symbol": ["BTC/USD"] * sample_count,
        "coordinate": np.arange(1, sample_count + 1),
    })
    config = {"advisors": {
        "momentum": {"features": [
            {"class": "Building", "within": 1, "keys": ["bull"]},
            {"class": "Stalling", "within": 1, "keys": ["flat"]},
        ]},
        "auction": {"features": [
            {"class": "BuyersBreakingThrough", "within": 1, "keys": ["bull"]},
            {"class": "Balanced", "within": 1, "keys": ["flat"]},
        ]},
        "participation": {"features": [
            {"class": "BroadLift", "within": 1, "keys": ["bull"]},
            {"class": "Unresolved", "within": 1, "keys": ["flat"]},
        ]},
    }}
    started = time.perf_counter()
    signals = council_signals(config, values, metadata)
    elapsed = time.perf_counter() - started
    print(
        f"benchmark_opportunity_council rows={sample_count} "
        f"signals={int(np.sum(signals))} elapsed={elapsed:.6f}s"
    )


def benchmark_candidate_rankings(sample_count=3353, candidate_count=404):
    random = np.random.default_rng(19)
    returns = random.normal(size=sample_count)
    columns = [f"metric/{index}" for index in range(candidate_count)]
    scores = pd.DataFrame(
        random.normal(size=(sample_count, candidate_count)), columns=columns,
    )
    scores[columns[0]] = returns + random.normal(scale=0.1, size=sample_count)
    metadata = pd.DataFrame({"gross_return": returns})
    config = {"advisors": {"momentum": {"features": [
        {"class": "Building"},
        {"class": "Reversing"},
    ]}}}
    started = time.perf_counter()
    rankings = candidate_rankings(
        config, columns, scores, metadata, np.arange(sample_count),
    )
    elapsed = time.perf_counter() - started
    supported = sum(len(ranking) for ranking in rankings.values())
    print(
        f"benchmark_candidate_rankings rows={sample_count} "
        f"candidates={candidate_count} supported={supported} "
        f"elapsed={elapsed:.6f}s"
    )


if __name__ == "__main__":
    unittest.main()
