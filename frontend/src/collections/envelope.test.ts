import { describe, expect, it } from "vitest";
import {
	ingestEnvelope,
	labelsAt,
	regionsAt,
	stores,
} from "./envelope";

describe("ingestEnvelope", () => {
	it("writes signal labels under the facet store key", () => {
		stores.hawkes.setState(() => ({}));

		ingestEnvelope({
			hawkes: {
				"BTC/USD": { spectral_radius: 0.9, maturity: 0.4 },
			},
		});

		expect(labelsAt(stores.hawkes.state, "BTC/USD")).toEqual({
			spectral_radius: 0.9,
			maturity: 0.4,
		});
	});

	it("merges region rows by symbol", () => {
		stores.regions.setState(() => ({}));

		ingestEnvelope({
			regions: {
				"ETH/USD": [{ key: 1, brightness: 0.8, counts: 2 }],
			},
		});

		expect(regionsAt(stores.regions.state, "ETH/USD")).toEqual([
			{ key: 1, brightness: 0.8, counts: 2 },
		]);
	});
});
