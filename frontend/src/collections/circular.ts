export type CircularBuffer<T> = {
	push: (value: T) => void;
	replaceTail: (value: T) => void;
	values: () => T[];
	length: () => number;
	capacity: () => number;
};

export const Circular = <T>(positions: number): CircularBuffer<T> => {
	const buffer = new Array<T>(positions);
	let start = 0;
	let count = 0;

	const push = (value: T) => {
		if (positions <= 0) {
			return;
		}

		const index = (start + count) % positions;

		if (count < positions) {
			buffer[index] = value;
			count++;
		} else {
			buffer[start] = value;
			start = (start + 1) % positions;
		}
	};

	const replaceTail = (value: T) => {
		if (positions <= 0) {
			return;
		}

		if (count === 0) {
			push(value);
			return;
		}

		buffer[(start + count - 1) % positions] = value;
	};

	const length = () => count;

	const values = () => {
		const result: T[] = [];

		for (let index = 0; index < count; index += 1) {
			result.push(buffer[(start + index) % positions]);
		}

		return result;
	};

	return { push, replaceTail, values, length, capacity: () => positions };
};
