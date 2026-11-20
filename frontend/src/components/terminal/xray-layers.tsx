const LAYER_NAMES = ["sensory", "micro", "meso", "macro"];

export const semanticLayerName = (index: number, count: number): string => {
	if (index <= 0) {
		return LAYER_NAMES[0] ?? "sensory";
	}

	if (index >= count - 1) {
		return "macro";
	}

	if (count === 3) {
		return "micro";
	}

	return LAYER_NAMES[index] ?? "latent";
};

export const layerCellsFromState = (
	state: unknown,
	cellCount = 16,
): number[] => {
	const values = Array.isArray(state)
		? state.filter((value): value is number => typeof value === "number")
		: [];

	if (values.length === 0 || cellCount <= 0) {
		return [];
	}

	if (values.length === cellCount) {
		return values;
	}

	if (values.length === 1) {
		return Array.from({ length: cellCount }, () => values[0] ?? 0);
	}

	return Array.from({ length: cellCount }, (_, index) => {
		const position = (index / Math.max(cellCount - 1, 1)) * (values.length - 1);
		const left = Math.floor(position);
		const right = Math.min(values.length - 1, left + 1);
		const ratio = position - left;

		return (values[left] ?? 0) * (1 - ratio) + (values[right] ?? 0) * ratio;
	});
};

export const keyedLayerCells = (
	label: string,
	values: number[],
): Array<{ key: string; value: number }> => {
	const occurrences = new Map<string, number>();
	return values.map((value) => {
		const valueKey = value.toFixed(6);
		const count = occurrences.get(valueKey) ?? 0;
		occurrences.set(valueKey, count + 1);
		return {
			key: `${label}-${valueKey}-${count}`,
			value,
		};
	});
};
