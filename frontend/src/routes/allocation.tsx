import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { causalStore } from "#/collections/causal";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import {
	AllocationMain,
	AllocationSidePanel,
	allocationSummary,
} from "#/components/terminal/allocation-side";

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

const RouteComponent = () => {
	const alloc = allocationSummary({
		focusSymbol: useSelector(appStore, (state) => state.focusSymbol),
		symbols: useSelector(instrumentsStore, (state) => state.symbols),
		balances: useSelector(balancesStore, (state) => state.balances),
		causal: useSelector(causalStore, (state) => state.causal),
		manifold: useSelector(manifoldStore, (state) => state.manifold),
		positions: useSelector(positionsStore, (state) => state.positions),
		resonance: useSelector(resonanceStore, (state) => state.resonance),
	});
	const orders = useSelector(ordersStore, (state) => state.orders);
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
					<AllocMetric
						label="Deployed"
						value={money(alloc.deployed)}
						accent
					/>
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
