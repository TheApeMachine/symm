import { createStore } from "@tanstack/react-store";

export type Balance = Record<string, unknown> & {
	asset: string;
	balance: number;
	available: number;
	reserved: number;
};

type BalanceFrame = Record<string, unknown>;

const asFrame = (value: unknown, path: string): BalanceFrame => {
	if (typeof value === "object" && value !== null && !Array.isArray(value)) {
		return value as BalanceFrame;
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

export const normalizeBalances = (balances: unknown): Balance[] => {
	if (!Array.isArray(balances)) {
		throw new TypeError("balances must be an array");
	}

	return balances.map((value, index) => {
		const path = `balances[${index}]`;
		const frame = asFrame(value, path);

		if (typeof frame.asset !== "string" || frame.asset.length === 0) {
			throw new TypeError(`${path}.asset must be a non-empty string`);
		}

		return {
			...frame,
			asset: frame.asset,
			balance: requiredFinite(frame.balance, `${path}.balance`),
			available: requiredFinite(frame.available, `${path}.available`),
			reserved: requiredFinite(frame.reserved, `${path}.reserved`),
		};
	});
};

export const balancesStore = createStore(
	{
		balances: [] as Balance[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (balances: unknown) =>
			setState(() => ({
				balances: normalizeBalances(balances),
				observed: true,
			})),
	}),
);
