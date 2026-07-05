import { bench, describe } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { Circular } from "#/collections/circular";
import { measurementOrigins } from "#/collections/measurements";
import { signalsSurfaceSources } from "./signals-surface";

describe("signalsSurfaceSources", () => {
	const measurements = {
		measurements: {
			...measurementOrigins(),
			customflow: Circular(50),
			customregime: Circular(50),
		},
		symbols: {},
	};

	bench("merges configured kernels with backend origins", () => {
		signalsSurfaceSources(DEFAULT_KERNELS, measurements);
	});
});
