import type { RingBuffer as RingBufferType } from "ring-buffer-ts";
import ringBufferPkg from "ring-buffer-ts";

type RingBufferModule = {
	RingBuffer?: typeof RingBufferType;
	default?: { RingBuffer?: typeof RingBufferType };
};

const pkg = ringBufferPkg as RingBufferModule | typeof RingBufferType;
const BaseRing = (
	"RingBuffer" in pkg && pkg.RingBuffer
		? pkg.RingBuffer
		: "default" in pkg && pkg.default?.RingBuffer
			? pkg.default.RingBuffer
			: pkg
) as typeof RingBufferType;

/** Adds publication positions to the existing storage implementation. */
export class RingBuffer<T> extends BaseRing<T> {
	end = 0;
	generation = 0;

	add(...items: T[]): void {
		super.add(...items);
		this.end += items.length;
	}

	clear(): void {
		super.clear();
		this.generation++;
	}

	fromArray(items: T[], resize = false): void {
		super.fromArray(items, resize);
		this.end += this.getBufferLength();
		this.generation++;
	}
}

/** Each subscriber owns a cursor; the ring's write slot is never a read cursor. */
export class RingCursor<T> {
	private ring?: RingBuffer<T>;
	private generation = -1;
	private position = 0;
	dropped = 0;

	read(ring: RingBuffer<T>, visit: (item: T) => void): void {
		const end = ring.end;
		const oldest = end - ring.getBufferLength();

		if (this.ring !== ring || this.generation !== ring.generation) {
			this.ring = ring;
			this.generation = ring.generation;
			this.position = oldest;
		}

		if (this.position < oldest) {
			this.dropped += oldest - this.position;
			this.position = oldest;
		}

		while (this.position < end) {
			const item = ring.get(this.position - oldest);

			if (item === undefined)
				throw new Error("Published ring entry is missing");

			visit(item);
			this.position++;
		}
	}
}
