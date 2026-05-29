import { bench, describe } from "vitest";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";

describe("ConfidenceDataProvider", () => {
	bench("hydrates gauge confidence snapshots", () => {
		ConfidenceDataProvider.reset();
		ConfidenceDataProvider.registerSource("hawkes", () => undefined);
		ConfidenceDataProvider.ingestSnapshot({
			hawkes: 0.3,
			fluid: 0.4,
			pumpdump: 0.5,
			causal: 0.6,
			depthflow: 0.7,
			leadlag: 0.8,
			liquidity: 0.9,
			sentiment: 0.2,
		});
	});
});
