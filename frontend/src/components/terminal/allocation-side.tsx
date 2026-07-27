import type { Balance } from "#/collections/types";
import type { CausalFrame } from "#/collections/types";
import type { Holding } from "#/collections/types";
import type { ManifoldFrame } from "#/collections/types";
import type { ResonanceFrame } from "#/collections/types";
import {
	causalConfidence,
	causalEntryBaseline,
	causalStrength,
} from "#/components/terminal/causal-view";
import { resonancePredict } from "#/components/terminal/decision-candidate";

type History<T> = { values: () => T[] };

type AllocationInput = {
	focusSymbol: string;
	symbols: string[];
	balances: Balance[];
	causal: Record<string, History<CausalFrame>>;
	manifold: Record<string, History<ManifoldFrame>>;
	holdings: Holding[];
	resonance: Record<string, History<ResonanceFrame>>;
};

type AllocationRow = {
	allocated: boolean;
	dotColor: string;
	inPlay: boolean;
	symbol: string;
	edge: number;
	edgeLeft: number;
	edgeWidth: number;
	notional: number;
	share: number;
	support: number;
	thesis: number;
	xPct: number;
};

export type AllocationSummary = {
	deployable: number;
	deployed: number;
	positionCount: number;
	mad: number;
	median: number;
	medianPct: number;
	quote: string;
	reserved: number;
	rows: AllocationRow[];
	threshold: number;
	thresholdPct: number;
};

const latest = <T,>(history?: History<T>): T | undefined =>
	history?.values().at(-1);

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const probability = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

const median = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const order = [...values].sort((left, right) => left - right);

	return order[Math.floor(order.length / 2)] ?? 0;
};

const quoteAsset = (symbol: string): string => symbol.split("/")[1] ?? "";

export const money = (value: number, quote: string): string =>
	`${value.toFixed(2)} ${quote}`;

const balanceAmount = (
	balances: Balance[],
	quote: string,
	field: "available" | "balance" | "reserved",
): number =>
	balances.reduce(
		(sum, balance) =>
			String(balance.asset).toUpperCase() === quote.toUpperCase()
				? sum + finite(balance[field])
				: sum,
		0,
	);

export const allocationSummary = ({
	focusSymbol,
	symbols,
	balances,
	causal,
	manifold,
	holdings,
	resonance,
}: AllocationInput): AllocationSummary => {
	const known = new Set([
		...Object.keys(causal),
		...Object.keys(manifold),
		...Object.keys(resonance),
	]);
	const orderedSymbols = [
		...symbols.filter((symbol) => known.has(symbol)),
		...[...known].filter((symbol) => !symbols.includes(symbol)).sort(),
	];
	const quote = quoteAsset(focusSymbol);
	const deployable = balanceAmount(balances, quote, "available");
	const reserved = balanceAmount(balances, quote, "reserved");
	const deployed = holdings
		.filter((holding) => quoteAsset(holding.symbol) === quote)
		.reduce((sum, holding) => sum + holding.qty * holding.mark, 0);
	const rows = orderedSymbols.map((symbol) => {
		const causalFrame = latest(causal[symbol]);
		const manifoldFrame = latest(manifold[symbol]);
		const resonanceFrame = latest(resonance[symbol]);
		const support = [causalFrame, manifoldFrame, resonanceFrame].filter(
			Boolean,
		).length;
		const thesis =
			Math.min(
				probability(resonancePredict(resonanceFrame) ?? 0),
				probability(causalConfidence(causalFrame)),
			) * Math.sqrt(Math.max(1, support));

		return {
			allocated: false,
			dotColor: "var(--f4)",
			edge: 0,
			edgeLeft: 0,
			edgeWidth: 0,
			inPlay:
				support >= 2 &&
				causalStrength(causalFrame) >= causalEntryBaseline(causalFrame),
			notional: 0,
			share: 0,
			support,
			symbol,
			thesis,
			xPct: 0,
		};
	});
	const scores = rows.map((row) => row.thesis);
	const med = median(scores);
	const dispersion =
		scores.length > 0
			? scores.reduce((sum, score) => sum + Math.abs(score - med), 0) /
				scores.length
			: 0;
	const threshold = med + dispersion;
	const sumPositive = scores.reduce(
		(sum, score) => sum + Math.max(0, score),
		0,
	);
	const lo = Math.min(...scores, threshold) * 0.92;
	const hi = Math.max(...scores, threshold) * 1.04;
	const span = hi - lo || 1;
	const pct = (value: number) =>
		Math.min(100, Math.max(0, ((value - lo) / span) * 100));
	const thresholdPct = pct(threshold);
	const medianPct = pct(med);

	const holdingBySymbol = new Map<string, Holding>();
	for (const holding of holdings) {
		holdingBySymbol.set(holding.symbol, holding);
	}

	const totalCapital = deployable + deployed;

	for (const row of rows) {
		row.edge = row.thesis - threshold;
		const holding = holdingBySymbol.get(row.symbol);

		if (holding !== undefined && holding.qty > 0 && holding.mark > 0) {
			row.notional = holding.qty * holding.mark;
			row.share = totalCapital > 0 ? row.notional / totalCapital : 0;
			row.allocated = true;
		} else {
			row.share =
				row.edge > 0 && row.thesis + sumPositive > 0
					? Math.min(0.2, row.edge / (row.thesis + sumPositive))
					: 0;
			row.allocated = false;
			row.notional = 0;
		}

		row.inPlay = row.inPlay || row.allocated || row.edge > 0;
		row.xPct = pct(row.thesis);
		row.edgeLeft = Math.min(thresholdPct, row.xPct);
		row.edgeWidth = row.edge > 0 ? Math.max(0, row.xPct - thresholdPct) : 0;
		row.dotColor = row.allocated
			? "var(--acc)"
			: row.inPlay
				? "var(--info)"
				: "var(--f4)";
	}

	return {
		deployable,
		deployed,
		mad: dispersion,
		median: med,
		medianPct,
		positionCount: holdings.length,
		quote,
		reserved,
		rows,
		threshold,
		thresholdPct,
	};
};

export const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

export const visibleRowSymbols = (alloc: AllocationSummary): string[] =>
	alloc.rows
		.filter((row) => row.allocated || row.inPlay)
		.map((row) => row.symbol);

export const allocatedSymbols = (alloc: AllocationSummary): string[] =>
	alloc.rows.filter((row) => row.allocated).map((row) => row.symbol);
