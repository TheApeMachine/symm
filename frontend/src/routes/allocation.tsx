import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { appStore } from "#/collections/app";
import type {
	Balance,
	CausalFrame,
	Holding,
	Instrument,
	ManifoldFrame,
	Order,
	ResonanceFrame,
} from "#/collections/types";
import {
	AllocationMain,
	AllocationSidePanel,
	allocationSummary,
} from "#/components/terminal/allocation-side";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

const AllocMetric = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex flex-col items-end gap-px">
		<span className="text-[9px] text-(--f4) uppercase tracking-widest">
			{label}
		</span>
		<span
			className="font-mono text-[13px] font-semibold"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</span>
	</div>
);

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
	const [symbols, setSymbols] = useState<string[]>([]);
	const [balances, setBalances] = useState<Balance[]>([]);
	const [holdings, setHoldings] = useState<Holding[]>([]);
	const [orders, setOrders] = useState<Order[]>([]);
	const [causal, setCausal] = useState<Record<string, History<CausalFrame>>>(
		{},
	);
	const [manifold, setManifold] = useState<
		Record<string, History<ManifoldFrame>>
	>({});
	const [resonance, setResonance] = useState<
		Record<string, History<ResonanceFrame>>
	>({});

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "instruments", key: "" },
			{ store: "balances", key: "" },
			{ store: "holdings", key: "" },
			{ store: "orders", key: "" },
			{ store: "causal", key: "" },
			{ store: "manifold", key: "" },
			{ store: "resonance", key: "" },
		],
		(buffers) => {
			const instruments = (buffers["instruments:"] ?? []) as Instrument[];
			setSymbols(instruments.map((instrument) => instrument.symbol).sort());
			setBalances((buffers["balances:"] ?? []) as Balance[]);
			setHoldings((buffers["holdings:"] ?? []) as Holding[]);
			setOrders((buffers["orders:"] ?? []) as Order[]);
			setCausal(asHistory((buffers["causal:"] ?? []) as CausalFrame[]));
			setManifold(asHistory((buffers["manifold:"] ?? []) as ManifoldFrame[]));
			setResonance(
				asHistory((buffers["resonance:"] ?? []) as ResonanceFrame[]),
			);
		},
		[online],
	);

	const alloc = allocationSummary({
		focusSymbol,
		symbols,
		balances,
		causal,
		manifold,
		holdings,
		resonance,
	});
	const money = (value: number) => `${value.toFixed(2)} ${alloc.quote}`;

	return (
		<div className="flex h-full min-w-[1080px] flex-col">
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
					<AllocMetric label="Deployable" value={money(alloc.deployable)} />
					<AllocMetric label="Deployed" value={money(alloc.deployed)} accent />
					<AllocMetric label="Positions" value={String(alloc.positionCount)} />
				</div>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
				<AllocationMain alloc={alloc} />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel alloc={alloc} orders={orders} />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
