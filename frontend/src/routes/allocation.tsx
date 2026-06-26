import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { balancesStore } from "#/collections/balances";
import { measurementsStore } from "#/collections/measurements";
import { playbookStore } from "#/collections/playbook";
import {
	AllocationMain,
	AllocationSidePanel,
	allocationModelFromStores,
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
	const balances = useSelector(balancesStore, (state) => state.frame);
	const evaluations = useSelector(playbookStore, (state) => state.evaluations);
	const readings = useSelector(measurementsStore, (state) => state);
	const alloc = allocationModelFromStores(balances, evaluations, readings);

	return (
		<div className="flex h-full min-w-[1080px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Edge-proportional sizing
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
						edge = thesis − median − mad · share = edge / (thesis + Σ positive)
						· notional = free × share
					</div>
				</div>
				<div className="ml-auto flex items-center gap-5">
					<AllocMetric
						label="Deployable"
						value={`${alloc.freeCash.toFixed(2)} ${alloc.quote}`}
					/>
					<AllocMetric
						label="Deployed"
						value={`${alloc.deployed.toFixed(2)} ${alloc.quote}`}
						accent
					/>
					<AllocMetric label="Positions" value={String(alloc.allocatedCount)} />
				</div>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
				<AllocationMain alloc={alloc} />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel alloc={alloc} />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
