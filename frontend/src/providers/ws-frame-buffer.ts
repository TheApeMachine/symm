const objectRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const frameRows = (value: unknown): unknown[] => {
	if (Array.isArray(value)) {
		return value;
	}

	const record = objectRecord(value);

	return record === null || symbolIdentity(record) === null
		? Object.values(record ?? {})
		: [record];
};

const positionIdentity = (value: unknown): string | null => {
	const position = objectRecord(value);

	if (position === null) {
		return null;
	}

	if (typeof position.id === "string" && position.id !== "") {
		return position.id;
	}

	const holding = objectRecord(position.holding);

	return typeof holding?.symbol === "string" && holding.symbol !== ""
		? holding.symbol
		: null;
};

const symbolIdentity = (value: unknown): string | null => {
	const frame = objectRecord(value);

	return typeof frame?.symbol === "string" && frame.symbol !== ""
		? frame.symbol
		: null;
};

/*
WorkerFrameBuffer coalesces sparse identity-owned deltas while a prior DRAW is
waiting to be painted. It deliberately retains nothing after take: the main
store remains authoritative across display frames, while the worker removes
superseded work before it crosses onto the UI thread.
*/
export class WorkerFrameBuffer {
	private readonly pending = new Map<string, unknown>();
	private readonly positions = new Map<string, unknown>();
	private readonly cognition = new Map<string, unknown>();
	private readonly resonance = new Map<string, unknown>();

	merge(frame: Record<string, unknown>) {
		for (const [key, value] of Object.entries(frame)) {
			if (key === "cognition") {
				this.mergeCognition(value);
				this.pending.set(key, null);
				continue;
			}

			if (key === "resonance") {
				this.mergeResonance(value);
				this.pending.set(key, null);
				continue;
			}

			if (key === "positions") {
				this.mergePositions(value);
				this.pending.set(key, null);
				continue;
			}

			this.pending.set(key, value);
		}
	}

	take(): Record<string, unknown> | null {
		if (this.pending.size === 0) {
			return null;
		}

		const frame = Object.fromEntries(
			Array.from(this.pending, ([key, value]) => {
				if (key === "cognition") {
					return [key, Object.fromEntries(this.cognition)];
				}

				if (key === "resonance") {
					return [key, Array.from(this.resonance.values())];
				}

				if (key === "positions") {
					return [key, Array.from(this.positions.values())];
				}

				return [key, value];
			}),
		);

		this.clear();

		return frame;
	}

	clear() {
		this.pending.clear();
		this.positions.clear();
		this.cognition.clear();
		this.resonance.clear();
	}

	private mergeCognition(value: unknown) {
		for (const [symbol, reading] of Object.entries(objectRecord(value) ?? {})) {
			this.cognition.set(symbol, reading);
		}
	}

	private mergeResonance(value: unknown) {
		for (const frame of frameRows(value)) {
			const identity = symbolIdentity(frame);

			if (identity !== null) {
				this.resonance.set(identity, frame);
			}
		}
	}

	private mergePositions(value: unknown) {
		for (const position of frameRows(value)) {
			const identity = positionIdentity(position);

			if (identity !== null) {
				this.positions.set(identity, position);
			}
		}
	}
}
