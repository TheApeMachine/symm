import { bench, describe } from "vitest";
import type { TradeObservation } from "#/types/thesis";
import { auditObservations, tradeObservationAuditRow } from "./dashboard-rail";

const observations: TradeObservation[] = Array.from(
	{ length: 128 },
	(_, index) => ({
		kind: index % 4 === 0 ? "lifecycle_transition" : "execution",
		symbol: index % 2 === 0 ? "BTC/USD" : "ETH/USD",
		status: index % 3 === 0 ? "filled" : "partially_filled",
		action: index % 5 === 0 ? "exit" : "enter",
		side: index % 2 === 0 ? "buy" : "sell",
		decision: index,
		at: `2026-07-12T04:05:${String(index % 60).padStart(2, "0")}Z`,
		quantity: "0.01",
		price: "61420",
	}),
);

describe("dashboard-rail audit", () => {
	bench("filters and formats trade journal audit rows", () => {
		const auditRows = auditObservations(observations);

		for (const observation of auditRows) {
			tradeObservationAuditRow(observation);
		}
	});
});
