import { bench, describe } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { regimeAxes } from "./regime-radar";
import { signalsSurfaceSources } from "./signals-surface";

// ponytail: static fixture timestamp and measurement shapes; upgrade path is varied timestamps and heterogeneous measurement payloads per kernel.
const at = "2026-07-15T09:00:00Z";
const measurement = {
	symbol: "BTC/USD",
	at,
	raw: 0,
	normalized: null,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: at, through: at },
};

measurementsStore.actions.updateFrame([
	{
		...measurement,
		source: "fluid",
		metric: "turbulent_score",
		raw: 0.8,
		normalized: 0.8,
	},
	{
		...measurement,
		source: "pumpdump",
		metric: "trend",
		raw: 0.6,
		normalized: 0.6,
	},
	{ ...measurement, source: "cvd", metric: "net", raw: 12 },
	{
		...measurement,
		source: "cvd",
		metric: "net_fraction",
		raw: 0.7,
		normalized: 0.7,
	},
	{
		...measurement,
		source: "cvd",
		metric: "balance",
		raw: 0.3,
		normalized: 0.3,
	},
]);

describe("signalsSurfaceSources", () => {
	bench("merges configured kernels with backend sources", () => {
		signalsSurfaceSources(DEFAULT_KERNELS, ["customflow", "customregime"]);
	});

	bench("projects typed measurements onto market regime axes", () => {
		regimeAxes(measurementsStore.state, ["fluid", "pumpdump", "cvd"]);
	});
});
