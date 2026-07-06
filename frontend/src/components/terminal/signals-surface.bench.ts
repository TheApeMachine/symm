import { bench, describe } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { Circular } from "#/collections/circular";
import type {
	Measurement,
	measurementsStore,
} from "#/collections/measurements";
import { signalsSurfaceSources } from "./signals-surface";

describe("signalsSurfaceSources", () => {
	const measurements: typeof measurementsStore.state = {
		measurements: {
			"BTC/USD": {
				customflow: Circular<Measurement>(50),
				customregime: Circular<Measurement>(50),
			},
		},
	};

	bench("merges configured kernels with backend sources", () => {
		signalsSurfaceSources(DEFAULT_KERNELS, measurements);
	});
});
