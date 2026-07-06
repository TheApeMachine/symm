export type CircularBuffer<T> = {
	push: (value: T) => void;
	values: () => T[];
	length: () => number;
};

export const Circular = <T>(positions: number): CircularBuffer<T> => {
	const buffer = new Array<T>(positions);
	let start = 0;
	let count = 0;

	const push = (value: T) => {
		if (positions <= 0) return;

		const index = (start + count) % positions;

		if (count < positions) {
			buffer[index] = value;
			count++;
		} else {
			buffer[start] = value;
			start = (start + 1) % positions;
		}
	};

	const length = () => {
		return count;
	};

	const values = () => {
		const result: T[] = [];

		for (let i = 0; i < count; i++) {
			result.push(buffer[(start + i) % positions]);
		}

		return result;
	};

	return { push, values, length };
};
