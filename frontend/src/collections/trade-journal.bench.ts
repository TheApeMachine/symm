import { bench, describe } from "vitest";
import { pushJournalObservations, tradeJournalStore } from "./trade-journal";

describe("tradeJournalStore", () => {
	bench("appends deduped observations into the circular journal", () => {
		tradeJournalStore.actions.updateFrame([
			{
				kind: "execution",
				symbol: "BTC/USD",
				status: "filled",
				at: `${performance.now()}`,
				decision: 0,
			},
		]);
	});
});
