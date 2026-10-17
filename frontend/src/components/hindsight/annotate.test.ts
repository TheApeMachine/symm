import { describe, expect, it } from "vitest";
import {
	annotationBody,
	emptyAnnotation,
	marksOf,
	nextMarker,
	placeMarker,
	snapTick,
} from "./annotate";

describe("annotate", () => {
	it("snaps a click to the nearest stored tick", () => {
		expect(snapTick([10, 20, 40], 28)).toBe(20);
		expect(snapTick([], 28)).toBeNull();
	});

	it("keeps each letter on its own tick", () => {
		const placed = placeMarker(
			placeMarker(emptyAnnotation(), "A", 10),
			"C",
			30,
		);
		expect(marksOf(placed)).toEqual([
			{ label: "A", at: 10 },
			{ label: "C", at: 30 },
		]);
		expect(nextMarker("C")).toBe("D");
	});

	it("saves only when A B C D are in tape order", () => {
		const ready = { A: 10, B: 20, C: 30, D: 40 };
		expect(annotationBody("7", "ES/USD", "up", ready)?.c).toBe("30");
		expect(annotationBody("7", "ES/USD", "up", { ...ready, B: 35 })).toBeNull();
		expect(
			annotationBody("7", "ES/USD", "up", { ...ready, D: null }),
		).toBeNull();
	});
});
