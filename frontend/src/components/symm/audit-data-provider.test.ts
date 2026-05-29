import { beforeEach, describe, expect, it } from "vitest";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";

describe("AuditDataProvider", () => {
	beforeEach(() => {
		AuditDataProvider.reset();
	});

	it("keeps newest audit rows first with compact summaries", () => {
		AuditDataProvider.ingest({
			event: "audit",
			audit_event: "trade_entry_eval",
			seq: 1,
			ts: "2026-05-29T01:02:03Z",
			symbol: "BTC/EUR",
			source: "cvd",
			edge: 0.012345,
			confidence: 0.72,
		});

		const row = AuditDataProvider.snapshot()[0];

		expect(row).toMatchObject({
			seq: 1,
			event: "trade_entry_eval",
			symbol: "BTC/EUR",
			source: "cvd",
		});
		expect(row?.summary).toContain("edge=0.01235");
		expect(row?.summary).toContain("confidence=0.7200");
	});

	it("ignores non-audit frames", () => {
		AuditDataProvider.ingest({
			event: "tick",
			ts: "2026-05-29T01:02:03Z",
		});

		expect(AuditDataProvider.snapshot()).toEqual([]);
	});
});
