import { describe, expect, it } from "vitest";
import { stampMs } from "./format";

describe("stampMs", () => {
	it("parses second-precision RFC3339", () => {
		expect(stampMs("2026-11-18T11:28:03Z")).toBe(
			Date.parse("2026-11-18T11:28:03Z"),
		);
	});

	it("parses Go RFC3339Nano by truncating to millis", () => {
		const nano = "2026-11-18T11:28:03.123456789Z";
		expect(stampMs(nano)).toBe(Date.parse("2026-11-18T11:28:03.123Z"));
		expect(stampMs(nano)).toBeGreaterThan(0);
	});

	it("orders a later nano close after an earlier second close", () => {
		const early = stampMs("2026-11-18T09:48:06Z");
		const late = stampMs("2026-11-18T11:28:03.987654321Z");
		expect(late).toBeGreaterThan(early);
	});

	it("returns 0 for empty or unusable values", () => {
		expect(stampMs("")).toBe(0);
		expect(stampMs(null)).toBe(0);
		expect(stampMs(undefined)).toBe(0);
		expect(stampMs("not-a-time")).toBe(0);
	});
});
