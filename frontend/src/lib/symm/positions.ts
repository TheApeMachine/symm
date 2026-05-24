import type { StatusEvent, StatusPosition } from "#/lib/symm/events";

export const sortOpenPositions = (
	positions: StatusPosition[],
): StatusPosition[] =>
	[...positions].sort((left, right) => {
		const leftOpenedAt = Date.parse(left.opened_at ?? "");
		const rightOpenedAt = Date.parse(right.opened_at ?? "");

		if (
			Number.isFinite(leftOpenedAt) &&
			Number.isFinite(rightOpenedAt) &&
			leftOpenedAt !== rightOpenedAt
		) {
			return leftOpenedAt - rightOpenedAt;
		}

		return left.symbol.localeCompare(right.symbol);
	});

export const positionSymbolsFromStatus = (
	status: StatusEvent | undefined,
): string[] => {
	const positions = status?.positions;
	if (!positions?.length) {
		return [];
	}

	return sortOpenPositions(positions).map((position) => position.symbol);
};
