import { describe, expect, it } from "vitest";
import {
	annotationBody,
	draftOf,
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

	it("drags the widest draft of the selected class", () => {
		const fields = (b: number, c: number) => ({
			a_tick: 1,
			b_tick: 2,
			c_tick: 3,
			d_tick: 4,
			b_price: b,
			c_price: c,
		});
		const picked = draftOf(
			[
				{ kind: "up", key: "draft/up/BTC/USD/1/1.bin", fields: fields(1, 1.1) },
				{ kind: "up", key: "draft/up/BTC/USD/1/2.bin", fields: fields(1, 1.4) },
				{
					kind: "up",
					key: "detections/up/BTC/USD/1/2.bin",
					fields: fields(1, 2),
				},
			],
			"up",
		);

		expect(picked?.key).toBe("draft/up/BTC/USD/1/2.bin");
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
