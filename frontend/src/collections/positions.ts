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

const optionalFinite = (value: unknown, fallback = 0): number => {
	if (value === null || value === undefined) {
		return fallback;
	}

	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	return Number.isFinite(number) ? number : fallback;
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

	const mark = optionalFinite(frame.mark);
	const entry = optionalFinite(frame.entry_price, mark);

	return {
		...rest,
		symbol: frame.symbol,
		qty: requiredFinite(frame.qty, `${path}.qty`),
		entry_price: entry,
		entry_fee: optionalFinite(frame.entry_fee),
		exit_fee: optionalFinite(frame.exit_fee),
		mark,
		pnl: optionalFinite(frame.pnl),
		return_pct: optionalFinite(frame.return_pct),
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
			setState((prev) => {
				// Balance.Publish sends a bare array (possibly empty). Only that
				// array shape is authoritative; incomplete non-array frames throw
				// in normalizePositions and must not clear observed inventory.
				if (!Array.isArray(positions)) {
					if (prev.observed && prev.positions.length > 0) {
						return prev;
					}

					throw new TypeError("positions must be an array");
				}

				return {
					positions: normalizePositions(positions),
					observed: true,
				};
			}),
	}),
);
