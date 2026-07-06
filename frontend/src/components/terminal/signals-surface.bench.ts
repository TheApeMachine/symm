import { bench, describe } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { Circular } from "#/collections/circular";
import type { MeasurementsState } from "#/collections/measurements";
import { signalsSurfaceSources } from "./signals-surface";

describe("signalsSurfaceSources", () => {
	const measurements: MeasurementsState = {
		measurements: {
			customflow: Circular(50),
			customregime: Circular(50),
		},
		symbols: {},
		sources: new Set(["customflow", "customregime"]),
		tick: 0,
	};

	bench("merges configured kernels with backend sources", () => {
		signalsSurfaceSources(DEFAULT_KERNELS, measurements);
	});
});
