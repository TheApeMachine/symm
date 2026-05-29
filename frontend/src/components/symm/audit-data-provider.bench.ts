import { bench, describe } from "vitest";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";

describe("AuditDataProvider", () => {
	bench("ingests realtime audit frames", () => {
		AuditDataProvider.reset();

		for (let index = 0; index < 64; index++) {
			AuditDataProvider.ingest({
				event: "audit",
				audit_event: "trade_entry_fill",
				seq: index,
				ts: "2026-05-29T01:02:03Z",
				symbol: "BTC/EUR",
				slot_eur: 10,
				confidence: 0.9,
			});
		}
	});
});
