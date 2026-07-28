import { createRef, useEffect } from "react";
import { appStore } from "#/collections/app";
import type {
	Balance,
	CausalFrame,
	Holding,
	Instrument,
	Position,
	ManifoldFrame,
	ResonanceFrame,
} from "#/collections/types";
import { frameRows } from "#/providers/frame-history";
import { paintAllocationSurface } from "./allocation-paint";
import { AllocationMain } from "./allocation-main";
import { AllocationSidePanel } from "./allocation-side-panel";
import { allocationSummary } from "./allocation-side";

const rootRef = createRef<HTMLDivElement>();

let lastInstruments: Instrument[] = [];
let lastBalances: Balance[] = [];
let lastPositions: Position[] = [];
let lastCausal: CausalFrame[] = [];
let lastManifold: ManifoldFrame[] = [];
let lastResonance: ResonanceFrame[] = [];

type History<T> = { values: () => T[] };

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, T>)
			: value != null
				? [value]
				: []) as T[];

const asHistory = <T extends { symbol: string }>(
	rows: T[],
): Record<string, History<T>> => {
	const map: Record<string, T[]> = {};

	for (const row of rows) {
		const bucket = map[row.symbol] ?? [];
		bucket.push(row);
		map[row.symbol] = bucket;
	}

	return Object.fromEntries(
		Object.entries(map).map(([symbol, values]) => [
			symbol,
			{ values: () => values },
		]),
	);
};

/*
paint rebuilds the allocation summary from the latest independently arriving
streams so a route mount and every subsequent frame see one coherent surface.
*/
const paint = () => {
	if (rootRef.current === null) {
		return;
	}

	const alloc = allocationSummary({
		focusSymbol: appStore.state.focusSymbol,
		symbols: lastInstruments.map((instrument) => instrument.symbol).sort(),
		balances: lastBalances,
		holdings: lastPositions
			.map((position) => position.holding)
			.filter((holding): holding is Holding => holding !== undefined && holding !== null),
		causal: asHistory(lastCausal),
		manifold: asHistory(lastManifold),
		resonance: asHistory(lastResonance),
	});

	paintAllocationSurface(rootRef.current, alloc);
};

/*
paintAllocationInstruments retains the current tradable universe for allocation.
*/
export const paintAllocationInstruments = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastInstruments = asRows<Instrument>(value);
	paint();
};

/*
paintAllocationBalances retains wallet state so navigation cannot erase capital.
*/
export const paintAllocationBalances = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastBalances = asRows<Balance>(value);
	paint();
};

/*
paintAllocationPositions retains the open portfolio for allocation calculations.
*/
export const paintAllocationPositions = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastPositions = (Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, Position>)
			: value != null
				? [value]
				: []) as Position[];
	paint();
};

export const paintAllocationHoldings = paintAllocationPositions;

/*
paintAllocationCausal retains the latest causal row for every observed symbol.
*/
export const paintAllocationCausal = (value: unknown, _focusSymbol: string) => {
	lastCausal = frameRows<CausalFrame>(value);
	paint();
};

/*
paintAllocationManifold retains the latest manifold row for every observed symbol.
*/
export const paintAllocationManifold = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastManifold = frameRows<ManifoldFrame>(value);
	paint();
};

/*
paintAllocationResonance retains model state instead of replacing it per tick.
*/
export const paintAllocationResonance = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastResonance = frameRows<ResonanceFrame>(value);
	paint();
};

/*
AllocationSurface owns the mounted allocation shell and restores retained state
immediately when the user navigates to it between sparse model frames.
*/
export const AllocationSurface = () => {
	useEffect(() => {
		paint();
	}, []);

	return (
		<div ref={rootRef} className="flex h-full min-w-270 flex-col">
			<div className="flex shrink-0 items-center gap-5.5 border-(--line) border-b bg-(--surface) px-4.5 py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Edge-proportional sizing
					</div>
					<div className="mt-0.75 font-mono text-[10px] text-(--f4)">
						edge = thesis - median - mad · share = edge / (thesis + sum
						positive) · notional = free x share
					</div>
				</div>
				<div className="ml-auto flex items-center gap-5">
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Deployable
						</span>
						<span
							data-alloc="deployable"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						/>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Deployed
						</span>
						<span
							data-alloc="deployed"
							className="font-mono text-[13px] font-semibold text-(--acc)"
						/>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Positions
						</span>
						<span
							data-alloc="positions"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						/>
					</div>
				</div>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
				<AllocationMain />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel />
				</div>
			</div>
		</div>
	);
};
