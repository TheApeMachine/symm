import { describe, expect, it } from "vitest";
import { temporalLevel } from "./gpu-stream";

describe("temporalLevel", () => {
	it("selects the first pyramid level bounded by physical pixels", () => {
		expect(temporalLevel(2048, 2048)).toBe(0);
		expect(temporalLevel(2048, 1024)).toBe(1);
		expect(temporalLevel(2048, 512)).toBe(2);
		expect(temporalLevel(2047, 512)).toBe(2);
	});
});
