import { createStore } from "@tanstack/react-store";

export type Position = Record<string, unknown> & {
	symbol: string;
	qty: number;
	entry_price: number;
	entry_fee: number;
	exit_fee: number;
	mark: number;
	pnl: number;
	return_pct: number;
	executions?: unknown[];
};

type PositionFrame = Record<string, unknown>;

const asFrame = (value: unknown, path: string): PositionFrame => {
	if (typeof value === "object" && value !== null && !Array.isArray(value)) {
		return value as PositionFrame;
	}

	throw new TypeError(`${path} must be an object`);
};

const requiredFinite = (value: unknown, path: string): number => {
	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	if (Number.isFinite(number)) {
		return number;
	}

	throw new TypeError(`${path} must be finite`);
};

const parseExecutions = (value: unknown): unknown[] | undefined => {
	if (value === undefined) {
		return undefined;
	}

	return Array.isArray(value) ? value : [];
};

const parsePosition = (value: unknown, index: number): Position => {
	const path = `positions[${index}]`;
	const frame = asFrame(value, path);

	if (typeof frame.symbol !== "string" || frame.symbol.length === 0) {
		throw new TypeError(`${path}.symbol must be a non-empty string`);
	}

	const { executions, ...rest } = frame;

	return {
		...rest,
		symbol: frame.symbol,
		qty: requiredFinite(frame.qty, `${path}.qty`),
		entry_price: requiredFinite(frame.entry_price, `${path}.entry_price`),
		entry_fee: requiredFinite(frame.entry_fee, `${path}.entry_fee`),
		exit_fee: requiredFinite(frame.exit_fee, `${path}.exit_fee`),
		mark: requiredFinite(frame.mark, `${path}.mark`),
		pnl: requiredFinite(frame.pnl, `${path}.pnl`),
		return_pct: requiredFinite(frame.return_pct, `${path}.return_pct`),
		executions: parseExecutions(executions),
	};
};

export const normalizePositions = (positions: unknown): Position[] => {
	if (!Array.isArray(positions)) {
		throw new TypeError("positions must be an array");
	}

	return positions.map(parsePosition);
};

export const positionsStore = createStore(
	{
		positions: [] as Position[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (positions: unknown) =>
			setState(() => ({
				positions: normalizePositions(positions),
				observed: true,
			})),
	}),
);
