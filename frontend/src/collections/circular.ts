export const Circular = (positions: number) => {
	const buffer = new Array<Record<string, unknown>>(positions);
	let start = 0;
	let count = 0;
	let snapshot: Record<string, unknown>[] | null = null;

	const push = (value: Record<string, unknown>) => {
		if (positions <= 0) return;

		const index = (start + count) % positions;

		if (count < positions) {
			buffer[index] = value;
			count++;
		} else {
			buffer[start] = value;
			start = (start + 1) % positions;
		}

		snapshot = null;
	};

	const values = () => {
		if (snapshot !== null) {
			return snapshot;
		}

		const result: Record<string, unknown>[] = [];

		for (let i = 0; i < count; i++) {
			result.push(buffer[(start + i) % positions]);
		}

		snapshot = result;

		return snapshot;
	};

	return { push, values };
};
