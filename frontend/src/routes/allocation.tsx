import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useRef, useState } from "react";
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
import {
	allocatedSymbols,
	allocationSummary,
	sameSymbols,
	visibleRowSymbols,
} from "#/components/terminal/allocation-side";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

type History<T> = { values: () => T[] };

const asHistory = <T extends { symbol: string }>(
	rows: T[],
): Record<string, History<T>> => {
	const map: Record<string, T[]> = {};

	for (const row of rows) {
		(map[row.symbol] ??= []).push(row);
	}

	return Object.fromEntries(
		Object.entries(map).map(([symbol, values]) => [
			symbol,
			{ values: () => values },
		]),
	);
};

const RouteComponent = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const rootRef = useRef<HTMLDivElement>(null);
	const [symbols, setSymbols] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "instruments", key: "" },
			{ store: "balances", key: "" },
			{ store: "holdings", key: "" },
			{ store: "causal", key: "" },
			{ store: "manifold", key: "" },
			{ store: "resonance", key: "" },
		],
		(buffers) => {
			const instruments = (buffers["instruments:"] ?? []) as Instrument[];
			const alloc = allocationSummary({
				focusSymbol,
				symbols: instruments.map((instrument) => instrument.symbol).sort(),
				balances: (buffers["balances:"] ?? []) as Balance[],
				holdings: (buffers["holdings:"] ?? []) as Holding[],
				causal: asHistory((buffers["causal:"] ?? []) as CausalFrame[]),
				manifold: asHistory((buffers["manifold:"] ?? []) as ManifoldFrame[]),
				resonance: asHistory(
					(buffers["resonance:"] ?? []) as ResonanceFrame[],
				),
			});
			const next = [
				...new Set([
					...visibleRowSymbols(alloc),
					...allocatedSymbols(alloc),
				]),
			].sort();

			setSymbols((previous) =>
				sameSymbols(previous, next) ? previous : next,
			);
			paintAllocationSurface(rootRef.current, alloc);
		},
		[online, focusSymbol],
	);

	return (
		<div ref={rootRef} className="flex h-full min-w-[1080px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Edge-proportional sizing
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
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
				<AllocationMain symbols={symbols} />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel symbols={symbols} />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
