import { beforeEach, describe, expect, it } from "vitest";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";

describe("AuditDataProvider", () => {
	beforeEach(() => {
		AuditDataProvider.reset();
	});

	it("keeps newest audit rows first with compact summaries", () => {
		AuditDataProvider.ingest({
			event: "audit",
			audit_event: "trade_entry_fill",
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
			event: "trade entry fill",
			symbol: "BTC/EUR",
			source: "cvd",
		});
		expect(row?.summary).toContain("edge=0.01235");
		expect(row?.summary).toContain("confidence=0.7200");
	});

	it("surfaces entry why, playbook, and perspectives in the summary", () => {
		AuditDataProvider.ingest({
			event: "audit",
			audit_event: "entry_submit",
			seq: 2,
			ts: "2026-05-29T01:02:04Z",
			symbol: "LOFI/EUR",
			source: "trader",
			reason: "pumpdump.vertical_ignition",
			why: "pumpdump.vertical_ignition",
			playbook: "pump",
			perspectives: ["pump"],
			conviction: 2.4,
			edge: 0.8,
			slot_eur: 3.13,
		});

		const row = AuditDataProvider.snapshot()[0];

		expect(row?.event).toBe("Entry submit");
		expect(row?.reason).toBe("pumpdump.vertical_ignition");
		expect(row?.summary).toContain("why=pumpdump.vertical_ignition");
		expect(row?.summary).toContain("playbook=pump");
		expect(row?.summary).toContain("perspectives=pump");
	});

	it("labels exit events with the desk reason", () => {
		AuditDataProvider.ingest({
			event: "audit",
			audit_event: "exit",
			seq: 3,
			ts: "2026-05-29T01:02:05Z",
			symbol: "BOBA/EUR",
			source: "trader",
			reason: "perspective TTL elapsed",
			why: "perspective TTL elapsed",
			actual_return: -0.0973,
			success: false,
		});

		const row = AuditDataProvider.snapshot()[0];

		expect(row?.event).toBe("Exit filled");
		expect(row?.reason).toBe("perspective TTL elapsed");
		expect(row?.summary).toContain("actual_return=-0.09730");
	});

	it("ignores non-audit frames", () => {
		AuditDataProvider.ingest({
			event: "tick",
			ts: "2026-05-29T01:02:03Z",
		});

		expect(AuditDataProvider.snapshot()).toEqual([]);
	});
});
