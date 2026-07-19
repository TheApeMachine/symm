import { bench, describe } from "vitest";
import type { Holding } from "#/collections/types";
import { auditHoldings, holdingAuditRow } from "./dashboard-rail";

const holdings: Holding[] = Array.from({ length: 64 }, (_, index) => ({
	symbol: `SYM-${index}/USD`,
	qty: index % 2 === 0 ? 0 : 1,
	entry_price: 1,
	entry_fee: 0,
	exit_fee: 0,
	mark: 1,
	pnl: index * 0.01,
	return_pct: 0.01,
	status: index % 2 === 0 ? "closed" : "open",
}));

describe("dashboard-rail audit holdings", () => {
	bench("filters and formats closed lots", () => {
		const closed = auditHoldings(holdings);

		for (const holding of closed) {
			holdingAuditRow(holding);
		}
	});
});
