import { bench, describe } from "vitest";
import { type HawkesTraceSample, hawkesTrace } from "./xray-hawkes-trace";

const samples: HawkesTraceSample[] = Array.from({ length: 50 }, (_, index) => ({
	at: BigInt(index) * 125_000_000n,
	intensity: 0.2 + (index % 7) * 0.03,
	postArrival: 0.7 + (index % 5) * 0.08,
	baseline: 0.2,
	decay: 1.4,
}));

describe("xray-hawkes-trace", () => {
	bench("hawkesTrace", () => {
		hawkesTrace(samples, 1200);
	});
});
