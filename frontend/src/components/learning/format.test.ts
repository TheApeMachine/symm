import { describe, expect, it } from "vitest";
import { rational } from "./format";

describe("rational", () => {
	it("formats exact account fractions only at the display boundary", () => {
		expect(rational("150")).toBe("150");
		expect(rational("4799/100")).toBe("47.99");
		expect(rational("0")).toBe("0");
		expect(rational("")).toBe("unavailable");
	});
});
