import { createFileRoute } from "@tanstack/react-router";
import { createRef } from "react";
import { appStore } from "#/collections/app";
import type {
	Balance,
	CausalFrame,
	Holding,
	Instrument,
	ManifoldFrame,
	ResonanceFrame,
} from "#/collections/types";
import { paintAllocationSurface } from "#/components/terminal/allocation-paint";
import {
	AllocationMain,
	AllocationSidePanel,
} from "#/components/terminal/allocation-panels";
import { allocationSummary } from "#/components/terminal/allocation-side";

const rootRef = createRef<HTMLDivElement>();

let lastInstruments: Instrument[] = [];
let lastBalances: Balance[] = [];
let lastHoldings: Holding[] = [];
let lastCausal: CausalFrame[] = [];
let lastManifold: ManifoldFrame[] = [];
let lastResonance: ResonanceFrame[] = [];

type History<T> = { values: () => T[] };

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as T[];

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
paint rebuilds the allocation summary from module caches and paints the shell.
*/
const paint = () => {
	const alloc = allocationSummary({
		focusSymbol: appStore.state.focusSymbol,
		symbols: lastInstruments.map((instrument) => instrument.symbol).sort(),
		balances: lastBalances,
		holdings: lastHoldings,
		causal: asHistory(lastCausal),
		manifold: asHistory(lastManifold),
		resonance: asHistory(lastResonance),
	});

	paintAllocationSurface(rootRef.current, alloc);
};

/*
paintAllocationInstruments refreshes allocation from the current DRAW instruments.
*/
export const paintAllocationInstruments = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastInstruments = asRows<Instrument>(value);
	paint();
};

/*
paintAllocationBalances refreshes allocation from the current DRAW balances.
*/
export const paintAllocationBalances = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastBalances = asRows<Balance>(value);
	paint();
};

/*
paintAllocationHoldings refreshes allocation from the current DRAW holdings.
*/
export const paintAllocationHoldings = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastHoldings = asRows<Holding>(value);
	paint();
};

/*
paintAllocationCausal refreshes allocation from retained causal history.
*/
export const paintAllocationCausal = (value: unknown, _focusSymbol: string) => {
	lastCausal = asRows<CausalFrame>(value);
	paint();
};

/*
paintAllocationManifold refreshes allocation from retained manifold history.
*/
export const paintAllocationManifold = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastManifold = asRows<ManifoldFrame>(value);
	paint();
};

/*
paintAllocationResonance refreshes allocation from retained resonance history.
*/
export const paintAllocationResonance = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastResonance = asRows<ResonanceFrame>(value);
	paint();
};

const RouteComponent = () => (
	<div ref={rootRef} className="flex h-full min-w-270 flex-col">
		<div className="flex shrink-0 items-center gap-5.5 border-(--line) border-b bg-(--surface) px-4.5 py-3">
			<div>
				<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
					Edge-proportional sizing
				</div>
				<div className="mt-0.75 font-mono text-[10px] text-(--f4)">
					edge = thesis - median - mad · share = edge / (thesis + sum positive)
					· notional = free x share
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

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
