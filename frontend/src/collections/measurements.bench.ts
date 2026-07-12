import { bench, describe } from "vitest";
import type { Measurement } from "#/types/measurement";
import { measurementEpochs, measurementRaw } from "./measurements";

const identities = [
	["event_count", ""],
	["event_count", "buy"],
	["event_count", "sell"],
	["arrival_rate", "buy"],
	["arrival_rate", "sell"],
	["conditional_intensity", "buy"],
	["conditional_intensity", "sell"],
	["baseline_intensity", "buy"],
	["baseline_intensity", "sell"],
	["excitation_amplitude", "buy_to_buy"],
	["excitation_amplitude", "sell_to_buy"],
	["excitation_amplitude", "buy_to_sell"],
	["excitation_amplitude", "sell_to_sell"],
	["decay_rate", ""],
	["kernel_memory", ""],
	["spectral_radius", ""],
	["hawkes_poisson_likelihood_delta", ""],
	["cross_self_likelihood_delta", ""],
	["immediate_expected_offspring", "buy"],
	["immediate_expected_offspring", "sell"],
	["expected_total_descendants", "buy"],
	["expected_total_descendants", "sell"],
] as const;

const measurements = ["2026-07-12T10:00:00Z", "2026-07-12T10:00:01Z"].flatMap(
	(at) =>
		identities.map(
			([metric, side], index): Measurement => ({
				source: "hawkes",
				metric,
				subject: "hawkes_process",
				stream: "trades",
				symbol: "BTC/USD",
				side,
				at,
				observedFrom: at,
				horizon: 0,
				unit: "dimensionless",
				raw: index,
				normalized: { value: 0, available: false },
				maturity: 1,
				uncertainty: { available: false },
				validity: { state: "provisional", readiness: "model" },
				scale: { kind: "observation_window", from: at, through: at },
			}),
		),
);

describe("measurement epoch readout", () => {
	bench("groups retained Hawkes records and reads a directional metric", () => {
		const epochs = measurementEpochs(measurements);
		measurementRaw(epochs.at(-1) ?? [], "conditional_intensity", "buy");
	});
});
