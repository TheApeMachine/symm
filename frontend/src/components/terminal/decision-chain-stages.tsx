
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
		<span
			data-paint={path}
			data-paint-format={format}
			data-paint-empty="—"
			className={className}
		/>
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

export const StructuralStage = () => (
	<DecisionStage title="1 · structural thesis" meta="conditioned market evidence">
		<TraceValue label="direction" path="direction" format="+.0f" />
		<TraceValue label="thesis score" path="thesisScore" format=".4f" />
		<TraceValue label="confidence" path="thesisConfidence" format=".1%" />
		<TraceValue
			label="support"
			path="thesisSupport"
			format=".4f"
			className="text-(--up)"
		/>
		<TraceValue
			label="contradiction"
			path="thesisContradiction"
			format=".4f"
			className="text-(--down)"
		/>
		<TraceValue label="conditions" path="thesisConditions" format=".4f" />
		<TraceValue label="predictive" path="predictiveStatus" />
		<TraceValue label="task skill" path="taskSkill" format=".3f" />
		<TraceValue label="supported horizon" path="forecastHorizon" format=".0f" />
		<TraceValue label="opportunity" path="opportunityType" />
		<TraceValue label="reserve" path="reserveReason" />
	</DecisionStage>
);

export const EvidenceStage = () => (
	<DecisionStage title="2 · evidence graph" meta="relations addressing the thesis">
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">supporting mass</span>
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
			<span className="text-(--f4)">contradicting mass</span>
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
		<TraceValue label="conditioning mass" path="trace.graphConditions" format=".3f" />
		<TraceValue label="balance" path="trace.thesisBalance" format=".4f" />
	</DecisionStage>
);

export const ExecutionStage = () => (
	<DecisionStage title="4 · execution + risk" meta="facts observable now">
		<TraceValue
			label="entry VWAP"
			path="entryCost.entryPrice"
			format=".8f"
			className="text-(--acc)"
		/>
		<TraceValue label="break-even" path="entryCost.breakEven" format=".8f" />
		<TraceValue label="round-trip fees" path="entryCost.roundTripFees" format=".6f" />
		<TraceValue label="spread" path="entryCost.spread" format=".8f" />
		<TraceValue label="impact" path="entryCost.impact" format=".8f" />
		<TraceValue label="quantity" path="proposedQuantity" format=".6f" />
		<TraceValue
			label="max loss"
			path="risk.max_loss"
			format=".4f"
			className="text-(--down)"
		/>
		<TraceValue label="risk distance" path="risk.risk_distance" format=".8f" />
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
