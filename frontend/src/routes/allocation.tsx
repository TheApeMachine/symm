import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { actionStore } from "#/collections/actions";
import { appStore } from "#/collections/app";
import {
	AllocationMain,
	AllocationSidePanel,
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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const actionState = useSelector(actionStore, (state) => state);
	const focusActions = actionState.actions[focusSymbol]?.values() ?? [];
	const actionCount = Object.values(actionState.actions).reduce(
		(sum, history) => sum + history.values().length,
		0,
	);
	const admitted = focusActions.filter((action) => action.verdict === "allow");
	const latest = focusActions.at(-1);

	return (
		<div className="flex h-full min-w-[1080px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Current trader admissions
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
						score, verdict, and fraction are backend decision fields · deployed
						comes from positions
					</div>
				</div>
				<div className="ml-auto flex items-center gap-5">
					<AllocMetric label="Focus" value={focusSymbol} />
					<AllocMetric label="Actions" value={String(actionCount)} />
					<AllocMetric label="Admitted" value={String(admitted.length)} accent />
					<AllocMetric label="Tick" value={String(latest?.tick ?? "—")} />
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

export const Route = createFileRoute("/allocation")({
	component: RouteComponent,
});
