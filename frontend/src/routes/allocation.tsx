import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { balancesStore } from "#/collections/balances";
import { AllocationSidePanel } from "#/components/terminal/allocation-side";
import { AllocationView } from "#/components/terminal/widgets";

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
	const assets =
		(balances?.asset as Array<Record<string, unknown>> | undefined) ?? [];
	const quote =
		(assets.find((a) => a.asset === "USD" || a.asset === "EUR")
			?.asset as string) ||
		(assets[0]?.asset as string) ||
		"USD";
	const cash = Number(
		assets.find((a) => a.asset === quote)?.balance ?? 0,
	);
	const positions = assets.filter(
		(a) => a.asset !== quote && Number(a.balance) > 0.00001,
	);

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
						value={`${cash.toFixed(2)} ${quote}`}
					/>
					<AllocMetric
						label="Positions"
						value={String(positions.length)}
						accent
					/>
				</div>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
				<AllocationView />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
