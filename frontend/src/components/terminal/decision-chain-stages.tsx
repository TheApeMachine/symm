import type { ReactNode } from "react";
import type { Decision } from "#/types/thesis";

const fmtn = (value: number | undefined, digits: number): string =>
	value === undefined ? "" : value.toFixed(digits);

const pct = (value: number | undefined, digits: number): string =>
	value === undefined ? "" : `${(value * 100).toFixed(digits)}%`;

const TraceValue = ({
	label,
	value,
	className = "text-(--f2)",
}: {
	label: string;
	value: string;
	className?: string;
}) => (
	<div className="flex items-center justify-between gap-2">
		<span className="text-(--f4)">{label}</span>
		<span className={className}>{value === "" ? "—" : value}</span>
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
			<span className="font-semibold text-[9px] text-(--f3) uppercase tracking-[0.12em]">{title}</span>
			<span className="text-[8px] text-(--f4)">{meta}</span>
		</div>
		<div className="flex flex-col gap-0.75">{children}</div>
	</div>
);

export const StructuralStage = ({ decision }: { decision: Decision }) => (
	<DecisionStage title="1 · structural thesis" meta="conditioned market evidence">
		<TraceValue label="direction" value={decision.direction === undefined ? "" : decision.direction.toFixed(0)} />
		<TraceValue label="thesis score" value={fmtn(decision.thesisScore, 4)} />
		<TraceValue label="confidence" value={pct(decision.thesisConfidence, 1)} />
		<TraceValue label="support" value={fmtn(decision.thesisSupport, 4)} className="text-(--up)" />
		<TraceValue label="contradiction" value={fmtn(decision.thesisContradiction, 4)} className="text-(--down)" />
		<TraceValue label="conditions" value={fmtn(decision.thesisConditions, 4)} />
		<TraceValue label="predictive" value={decision.predictiveStatus || ""} />
		<TraceValue label="task skill" value={fmtn(decision.taskSkill, 3)} />
		<TraceValue label="supported horizon" value={decision.forecastHorizon === undefined ? "" : decision.forecastHorizon.toFixed(0)} />
		<TraceValue label="opportunity" value={decision.opportunityType || ""} />
		<TraceValue label="reserve" value={decision.reserveReason || ""} />
	</DecisionStage>
);

export const EvidenceStage = ({ decision }: { decision: Decision }) => (
	<DecisionStage title="2 · evidence graph" meta="relations addressing the thesis">
		<TraceValue label="supporting mass" value={fmtn(decision.trace?.graphSupports, 3)} className="text-(--up)" />
		<TraceValue label="contradicting mass" value={fmtn(decision.trace?.graphContradicts, 3)} className="text-(--down)" />
		<TraceValue label="conditioning mass" value={fmtn(decision.trace?.graphConditions, 3)} />
		<TraceValue label="balance" value={fmtn(decision.trace?.thesisBalance, 4)} />
	</DecisionStage>
);

export const ExecutionStage = ({ decision }: { decision: Decision }) => (
	<DecisionStage title="5 · execution + risk" meta="facts observable now">
		<TraceValue label="entry VWAP" value={decision.entryCost?.entryPrice ?? ""} className="text-(--acc)" />
		<TraceValue label="break-even" value={decision.entryCost?.breakEven ?? ""} />
		<TraceValue label="round-trip fees" value={decision.entryCost?.roundTripFees ?? ""} />
		<TraceValue label="spread" value={decision.entryCost?.spread ?? ""} />
		<TraceValue label="impact" value={decision.entryCost?.impact ?? ""} />
		<TraceValue label="quantity" value={decision.proposedQuantity} />
		<TraceValue label="max loss" value={decision.risk?.max_loss == null ? "" : String(decision.risk.max_loss)} className="text-(--down)" />
		<TraceValue label="risk distance" value={decision.risk?.risk_distance == null ? "" : String(decision.risk.risk_distance)} />
		<div className="flex justify-between gap-2 text-(--f4)">
			<span>slots</span>
			<span>
				<b className="font-normal text-(--f2)">{String(decision.openPositions)}</b> /{" "}
				<b className="font-normal text-(--f2)">{String(decision.slotCapacity)}</b>
			</span>
		</div>
		<div className="flex justify-between gap-2">
			<span className="text-(--f3)">{decision.allocationClass || ""}</span>
			<span className="truncate text-(--f4)">{decision.cause || ""}</span>
		</div>
	</DecisionStage>
);