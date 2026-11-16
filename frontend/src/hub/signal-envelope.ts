import { DEFAULT_KERNELS } from "#/collections/app";

const kernelKeys = new Set<string>(DEFAULT_KERNELS);

/*
Hub Publish(ctx, focus) sends one reading under ctx, not {symbol: reading}.
ui.Focus during the pipeline still sends symbol-keyed maps — detect by key shape.
*/
export const isSymbolKeyedMap = (value: Record<string, unknown>): boolean => {
	const keys = Object.keys(value);
	if (keys.length === 0) {
		return true;
	}

	return keys.some((key) => key.includes("/"));
};

export const normalizeKernelStoreValue = (
	key: string,
	value: unknown,
	focus: string,
): unknown => {
	if (!kernelKeys.has(key) || value == null || focus === "") {
		return value;
	}

	if (typeof value !== "object" || Array.isArray(value)) {
		return value;
	}

	const record = value as Record<string, unknown>;
	if (isSymbolKeyedMap(record)) {
		return value;
	}

	return { [focus]: record };
};