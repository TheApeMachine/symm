export function formatFluidScalar(value: number): string {
	if (!Number.isFinite(value)) {
		return "—";
	}

	const magnitude = Math.abs(value);

	if (magnitude === 0) {
		return "0";
	}

	return value.toFixed(2);
}
