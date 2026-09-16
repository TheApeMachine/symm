import { bench, describe } from "vitest";
import { RingBuffer, RingCursor } from "./ring";

describe("RingCursor.read", () => {
	const ring = new RingBuffer<number>(50);
	const cursor = new RingCursor<number>();
	let sequence = 0;
	let received = 0;
	bench("published batch through wrapped ring", () => {
		ring.add(sequence++, sequence++, sequence++);
		cursor.read(ring, (value) => {
			received = value;
		});
		if (received !== sequence - 1) throw new Error("Unread publication");
	});
});
