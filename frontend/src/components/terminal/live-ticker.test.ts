import { describe, expect, it } from "vitest";
import {
	engineClockText,
	engineUptimeText,
} from "#/components/terminal/live-ticker";

describe("engineClockText", () => {
	it("formats UTC wall time from the given instant", () => {
		expect(engineClockText(new Date("2026-07-20T08:37:12.000Z"))).toBe(
			"08:37:12 UTC",
		);
	});
});

describe("engineUptimeText", () => {
	it("prefixes formatUptime for the engine footer", () => {
		expect(engineUptimeText(null)).toBe("uptime —");
		expect(engineUptimeText(Date.now() - 65_000)).toMatch(/^uptime /);
	});
});
