import { describe, expect, it } from "vitest";
import { RingBuffer, RingCursor } from "./ring";

describe("RingCursor.read", () => {
	it("consumes batches once through warmup, wraparound, and independent readers", () => {
		const ring = new RingBuffer<number>(3);
		const first = new RingCursor<number>();
		const second = new RingCursor<number>();
		const values: number[] = [];
		ring.add(1, 2);
		first.read(ring, (value) => values.push(value));
		first.read(ring, (value) => values.push(value));
		ring.add(3, 4);
		first.read(ring, (value) => values.push(value));
		expect(values).toEqual([1, 2, 3, 4]);
		const other: number[] = [];
		second.read(ring, (value) => other.push(value));
		expect(other).toEqual([2, 3, 4]);
		expect(first.dropped).toBe(0);
	});

	it("accounts for overwritten unread entries and handles stream resets", () => {
		const ring = new RingBuffer<number>(2);
		const cursor = new RingCursor<number>();
		const values: number[] = [];
		ring.add(1);
		cursor.read(ring, (value) => values.push(value));
		ring.add(2, 3, 4);
		cursor.read(ring, (value) => values.push(value));
		expect(values).toEqual([1, 3, 4]);
		expect(cursor.dropped).toBe(1);
		ring.clear();
		ring.add(5);
		cursor.read(ring, (value) => values.push(value));
		const next = new RingBuffer<number>(2);
		next.add(6);
		cursor.read(next, (value) => values.push(value));
		expect(values).toEqual([1, 3, 4, 5, 6]);
	});
});
