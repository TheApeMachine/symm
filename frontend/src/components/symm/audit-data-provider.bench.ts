import { bench, describe } from "vitest";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";

describe("AuditDataProvider", () => {
	bench("ingests realtime audit frames", () => {
		AuditDataProvider.reset();

		for (let index = 0; index < 64; index++) {
			AuditDataProvider.ingest({
				event: "audit",
				audit_event: "trade_entry_skip",
				seq: index,
				ts: "2026-05-29T01:02:03Z",
				symbol: "BTC/EUR",
				reason: "edge_below_threshold",
				edge: 0.004,
			});
		}
	});
});
