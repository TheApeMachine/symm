import { describe, expect, it } from "vitest";
import {
	meterInlineLabelClass,
	meterInlineValueClass,
	meterTrackVariants,
	meterVariants,
} from "./meter";
import type { Size } from "./types";

const meterSizes: Size[] = ["xxs", "xs", "s", "m", "lg", "xl", "xxl"];

describe("meterTrackVariants", () => {
	it("applies semantic tone and size classes to the track", () => {
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"[--meter-tone:var(--success)]",
		);
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"h-1.5",
		);
	});
});

describe("meterVariants", () => {
	it("applies layout classes", () => {
		expect(meterVariants({ layout: "inline" })).toBe("flex items-center gap-2");
	});
});

describe("meter inline typography", () => {
	it("defines label and value classes for every size", () => {
		for (const size of meterSizes) {
			expect(meterInlineLabelClass[size]).toMatch(/shrink-0/);
			expect(meterInlineLabelClass[size]).toMatch(/text-/);
			expect(meterInlineValueClass[size]).toMatch(
				/shrink-0 text-right font-mono/,
			);
			expect(meterInlineValueClass[size]).toMatch(/text-/);
		}
	});

	it("keeps inline value wide enough for stop words on one line", () => {
		expect(meterInlineValueClass.s).toContain("w-[4rem]");
		expect(meterInlineValueClass.s).toContain("whitespace-nowrap");
		expect(meterInlineLabelClass.s).toContain("whitespace-nowrap");
	});
});
