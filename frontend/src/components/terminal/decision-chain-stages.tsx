import type { ReactNode } from "react";
import type { DecisionT } from "#/providers/telemetry/telemetry/decision";

/*
The decision stages, as the current architecture actually decides.

These panels used to show a "structural thesis" (score, support, contradiction,
conditions) and an "evidence graph" (supporting/contradicting/conditioning
mass). Neither exists any more: types.Decision carries no such field, nothing in
Go writes them, and the wire slots left behind are permanently zero — which is
why every symbol read thesis=0.0000 support=0.0000 forever. Showing a constant
zero as if it were a measurement is worse than showing nothing, because it
invites the reader to reason about it.

What decides now is the chain in MCTS.md §5: an opportunity reaches PhaseArmed,
the War Room deliberates the seven market moves, the causal search plans over
them, and allocation prices the entry. So the stages are the stages of that
chain, and every value below is one the planner actually sets.
*/

const text = (value: string | Uint8Array | null | undefined): string =>
	typeof value === "string" ? value : "";

const num = (value: number | undefined, digits: number): string =>
	value === undefined || !Number.isFinite(value) ? "" : value.toFixed(digits);

const pct = (value: number | undefined, digits: number): string =>
	value === undefined || !Number.isFinite(value)
		? ""
		: `${(value * 100).toFixed(digits)}%`;

const TraceValue = ({
	label,
	value,
	className = "text-(--f2)",
	title,
}: {
	label: string;
	value: string;
	className?: string;
	title?: string;
}) => (
	<div className="flex items-center justify-between gap-2" title={title}>
		<span className="text-(--f4)">{label}</span>
		<span className={value === "" ? "text-(--f4)" : className}>
			{value === "" ? "—" : value}
		</span>
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

/*
PrecursorStage is the entry gate of MCTS.md §1.2: position while the precursor
is armed, never once vertical ignition has printed. The phase is therefore the
single most decision-relevant fact on the row.
*/
export const PrecursorStage = ({ decision }: { decision: DecisionT }) => {
	const phase = text(decision.opportunityPhase);
	const armed = phase === "armed";

	return (
		<DecisionStage title="1 · precursor" meta="position while armed, never on ignition">
			<TraceValue
				label="archetype"
				value={text(decision.opportunityType)}
				title="The opportunity type the tracker recognised for this symbol."
			/>
			<TraceValue
				label="phase"
				value={phase}
				className={armed ? "text-(--up)" : "text-(--f2)"}
				title="Entry is admitted only at PhaseArmed and refused once PhaseIgnition prints."
			/>
			<TraceValue
				label="tracked"
				value={decision.opportunity ? "yes" : "no"}
			/>
			<TraceValue
				label="direction"
				value={num(decision.direction, 0)}
			/>
			<TraceValue
				label="horizon"
				value={
					decision.forecastHorizon === undefined
						? ""
						: String(decision.forecastHorizon)
				}
				title="Rollout depth, in ticker steps, the search planned over."
			/>
		</DecisionStage>
	);
};

/*
ReadinessStage names why a round did or did not reach the search. This is the
stage that was invisible before: a round that stopped early stopped for exactly
one declared reason, and that reason is the most useful thing on the surface
when nothing is trading.
*/
export const ReadinessStage = ({ decision }: { decision: DecisionT }) => {
	const status = text(decision.predictiveStatus);
	const resolved = decision.predictiveReady;

	return (
		<DecisionStage title="2 · readiness" meta="how far this round got">
			<TraceValue
				label="status"
				value={status}
				className={resolved ? "text-(--up)" : "text-(--warn)"}
				title="The exact stage this round reached, or the reason it stopped."
			/>
			<TraceValue
				label="searched"
				value={resolved ? "yes" : "no"}
				className={resolved ? "text-(--up)" : "text-(--f2)"}
			/>
			<TraceValue
				label="transition"
				value={text(decision.forecastSource)}
				title="Which model supplied the market transition: resonance forecast, or the council's own distribution."
			/>
			<TraceValue
				label="model"
				value={text(decision.forecastModel)}
			/>
			<TraceValue
				label="rollouts"
				value={
					decision.calibrationCount === undefined
						? ""
						: String(decision.calibrationCount)
				}
				title="Real rollout visits the search spent on this round."
			/>
			<TraceValue label="confidence" value={pct(decision.confidence, 1)} />
		</DecisionStage>
	);
};

/*
ExecutionStage is the economics of the entry as priced now. Nothing here is a
forecast: these are the fees, spread and impact the allocation actually
computed, so an admitted entry can be checked against what it would cost.
*/
export const ExecutionStage = ({ decision }: { decision: DecisionT }) => (
	<DecisionStage title="3 · execution + risk" meta="facts observable now">
		<TraceValue
			label="reference price"
			value={text(decision.referencePrice)}
			className="text-(--acc)"
		/>
		<TraceValue label="expected fees" value={text(decision.expectedFees)} />
		<TraceValue label="expected spread" value={text(decision.expectedSpread)} />
		<TraceValue label="expected impact" value={text(decision.expectedImpact)} />
		<TraceValue
			label="adverse selection"
			value={text(decision.adverseSelection)}
			className="text-(--down)"
			title="The informed-flow cost the causal head attributed to trading here."
		/>
		<TraceValue label="quantity" value={text(decision.proposedQuantity)} />
		<TraceValue label="notional" value={text(decision.proposedNotional)} />
		<TraceValue
			label="allocation"
			value={text(decision.allocationClass)}
		/>
	</DecisionStage>
);
