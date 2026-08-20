export type CircularBuffer<T> = {
	push: (value: T) => void;
	array: () => unknown[];
	replaceTail: (value: T) => void;
	values: () => T[];
	latest: () => T | undefined;
	forEach: (visit: (value: T, index: number) => void) => void;
	physical: () => { buffer: Array<T>; start: number; count: number };
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

	const array = (): unknown[] => values();

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
	const latest = () =>
		count === 0 ? undefined : buffer[(start + count - 1) % positions];

	const forEach = (visit: (value: T, index: number) => void) => {
		for (let index = 0; index < count; index += 1) {
			visit(buffer[(start + index) % positions] as T, index);
		}
	};

	const values = () => {
		const result: T[] = [];

		for (let index = 0; index < count; index += 1) {
			result.push(buffer[(start + index) % positions]);
		}

		return result;
	};

	return {
		push,
		array,
		replaceTail,
		values,
		latest,
		forEach,
		physical: () => ({ buffer, start, count }),
		length,
		capacity: () => positions,
	};
};

/*
latestOf returns the newest retained value in a circular buffer.
*/
export const latestOf = <T>(buffer?: CircularBuffer<T>): T | undefined =>
	buffer?.latest();

/*
latestValues returns the newest retained value for each key, sorted by key.
*/
export const latestValues = <T>(
	index: Record<string, CircularBuffer<T>>,
): T[] => {
	const result: T[] = [];

	for (const buffer of Object.values(index)) {
		const value = buffer.latest();

		if (value !== undefined) {
			result.push(value);
		}
	}

	return result;
};
