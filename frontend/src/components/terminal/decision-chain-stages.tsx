import type { ReactNode } from "react";

export const TraceValue = ({
	label,
	path,
	format,
	className = "text-(--f2)",
}: {
	label: string;
	path: string;
	format?: string;
	className?: string;
}) => (
	<div className="flex items-center justify-between gap-2">
		<span className="text-(--f4)">{label}</span>
		<span data-paint={path} data-paint-format={format} className={className} />
	</div>
);

export const DecisionStage = ({
	title,
	meta,
	children,
}: {
	title: string;
	meta: string;
	children: ReactNode;
}) => (
	<div className="min-w-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-2">
		<div className="mb-1.5 flex items-center justify-between gap-2">
			<span className="font-semibold text-[9px] text-(--f3) uppercase tracking-[0.12em]">
				{title}
			</span>
			<span className="text-[8px] text-(--f4)">{meta}</span>
		</div>
		<div className="flex flex-col gap-0.75">{children}</div>
	</div>
);

export const ForecastStage = () => (
	<DecisionStage title="1 · forecast" meta="net edge before sizing">
		<TraceValue label="return" path="expectedReturn" format=".6f" />
		<TraceValue label="fees" path="expectedFees" format=".6f" />
		<TraceValue label="spread" path="expectedSpread" format=".6f" />
		<TraceValue label="impact" path="expectedImpact" format=".6f" />
	</DecisionStage>
);

export const EvidenceStage = () => (
	<DecisionStage title="2 · evidence graph" meta="edge mass used by search">
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">supporting edges</span>
			<span
				data-paint="trace.graphSupports"
				data-paint-format=".3f"
				className="text-(--up)"
			/>
		</div>
		<div className="h-1.25 overflow-hidden rounded-[3px] bg-(--line)">
			<div
				data-set="trace.graphSupports"
				data-set-scale="domain-percent"
				data-set-domain="trace.graphSupports,trace.graphContradicts"
				data-target="style.width"
				className="h-full bg-(--up)"
			/>
		</div>
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">contradicting edges</span>
			<span
				data-paint="trace.graphContradicts"
				data-paint-format=".3f"
				className="text-(--down)"
			/>
		</div>
		<div className="h-1.25 overflow-hidden rounded-[3px] bg-(--line)">
			<div
				data-set="trace.graphContradicts"
				data-set-scale="domain-percent"
				data-set-domain="trace.graphSupports,trace.graphContradicts"
				data-target="style.width"
				className="h-full bg-(--down)"
			/>
		</div>
	</DecisionStage>
);

export const CapitalStage = () => (
	<DecisionStage title="4 · capital + slots" meta="final decision">
		<div className="rounded-xs bg-[color-mix(in_srgb,var(--down)_12%,transparent)] px-1.5 py-1 text-(--down)">
			<div className="flex items-center justify-between gap-2">
				<span>flow haircut</span>
				<span data-paint="allocation_haircut" data-paint-format=".1%" />
			</div>
			<div
				data-paint="allocation_haircut_reason"
				className="mt-0.5 truncate text-[8px] text-(--f3)"
			/>
		</div>
		<TraceValue
			label="notional"
			path="proposedNotional"
			format=".2f"
			className="text-(--acc)"
		/>
		<TraceValue label="quantity" path="proposedQuantity" format=".6f" />
		<TraceValue
			label="max loss"
			path="risk.max_loss"
			format=".4f"
			className="text-(--down)"
		/>
		<TraceValue label="risk distance" path="risk.risk_distance" format=".6f" />
		<div className="flex justify-between gap-2 text-(--f4)">
			<span>slots</span>
			<span>
				<b data-paint="openPositions" className="font-normal text-(--f2)" /> /{" "}
				<b data-paint="slotCapacity" className="font-normal text-(--f2)" />
			</span>
		</div>
		<div className="flex justify-between gap-2">
			<span data-paint="allocationClass" className="text-(--f3)" />
			<span data-paint="cause" className="truncate text-(--f4)" />
		</div>
	</DecisionStage>
);
