import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	evidenceMeaning,
	evidenceValue,
	type FrozenEntryDecision,
	readEntryDecision,
} from "./entry-decision-model";

const display = (value: string): string => value || "—";

const entryTime = (nanoseconds: bigint): string => {
	if (nanoseconds <= 0n) {
		return "—";
	}

	return new Date(Number(nanoseconds / 1_000_000n)).toLocaleString();
};

const confidenceText = (confidence: number): string =>
	`${(confidence * 100).toFixed(1)}%`;

const directionText = (direction: number): string => {
	if (direction > 0) {
		return "upward move";
	}

	if (direction < 0) {
		return "downward move";
	}

	return "no directional lean";
};

const SnapshotCard = ({
	index,
	title,
	value,
	explanation,
}: {
	index: number;
	title: string;
	value: React.ReactNode;
	explanation: string;
}) => (
	<div className="relative min-w-0 rounded-[4px] border border-(--line) bg-(--sunken) px-3 py-3">
		<div className="mb-2 flex items-center gap-2">
			<span className="flex size-5 items-center justify-center rounded-full border border-(--line2) font-mono text-[9px] text-(--acc)">
				{index}
			</span>
			<Typography.Label size="s" tone="f2">
				{title}
			</Typography.Label>
		</div>
		<div className="font-mono text-[12px] text-(--f1)">{value}</div>
		<p className="mt-1.5 text-[10px] text-(--f4) leading-relaxed">
			{explanation}
		</p>
	</div>
);

const Fact = ({
	label,
	value,
	help,
}: {
	label: string;
	value: string;
	help: string;
}) => (
	<div className="border-(--line) border-b px-3 py-2 last:border-b-0">
		<Flex.Row align="baseline" justify="between" gap={3}>
			<span className="font-mono text-[9px] text-(--f3)">{label}</span>
			<span className="text-right font-mono text-[10px] text-(--f1)">
				{display(value)}
			</span>
		</Flex.Row>
		<p className="mt-0.5 text-[9px] text-(--f4) leading-relaxed">{help}</p>
	</div>
);

const DecisionPath = ({ decision }: { decision: FrozenEntryDecision }) => {
	const expected = decision.evidence.find(
		(entry) => entry.key === "return:expected_log",
	)?.value;
	const breakEven = decision.evidence.find(
		(entry) => entry.key === "return:break_even_log",
	)?.value;
	const costGate =
		expected !== undefined && breakEven !== undefined
			? `${(Math.expm1(expected) * 100).toFixed(2)}% expected vs ${(Math.expm1(breakEven) * 100).toFixed(2)}% needed`
			: "Cost boundary recorded below";

	return (
		<div className="grid grid-cols-2 gap-2 xl:grid-cols-4">
			<SnapshotCard
				index={1}
				title="Opportunity appeared"
				value={
					<>
						{display(decision.opportunityType)}
						<span className="text-(--f4)">
							{" "}
							· {display(decision.opportunityPhase)}
						</span>
					</>
				}
				explanation="The opportunity tracker recognized this market shape and recorded its phase. This is the setup that earned further evaluation—not an entry by itself."
			/>
			<SnapshotCard
				index={2}
				title="Forecast became usable"
				value={
					<span
						className={
							decision.predictiveReady ? "text-(--up)" : "text-(--warn)"
						}
					>
						{display(decision.predictiveStatus)}
					</span>
				}
				explanation={`The adaptive model had ${decision.calibrationCount.toString()} prior calibrations and looked ${decision.forecastHorizon.toString()} ticker observations ahead.`}
			/>
			<SnapshotCard
				index={3}
				title="Expected move cleared costs"
				value={<span className="text-(--up)">{costGate}</span>}
				explanation="The forecast center had to exceed the fee-inclusive break-even return. If it had merely predicted an upward move without covering costs, cash would have won instead."
			/>
			<SnapshotCard
				index={4}
				title="Risk-sized order admitted"
				value={`${display(decision.proposedNotional)} USD · ${display(decision.proposedQuantity)} units`}
				explanation="The final quantity was capped by visible liquidity, venue minimums, available cash, and the loss geometry recorded with this decision."
			/>
		</div>
	);
};

const Confidence = ({ decision }: { decision: FrozenEntryDecision }) => (
	<Section fit="content" surface="sunken">
		<Section.Header title="How convinced was the system?" size="s" rule />
		<Section.Body>
			<div className="px-3 py-3">
				<Flex.Row align="baseline" justify="between">
					<span className="font-mono text-[10px] text-(--f3)">
						probability of clearing entry costs
					</span>
					<span className="font-mono text-[15px] text-(--acc)">
						{confidenceText(decision.confidence)}
					</span>
				</Flex.Row>
				<div className="mt-2 h-2 overflow-hidden rounded-full bg-(--line)">
					<div
						className="h-full rounded-full bg-(--acc)"
						style={{ width: `${decision.confidence * 100}%` }}
					/>
				</div>
				<p className="mt-2 text-[10px] text-(--f4) leading-relaxed">
					Read this as the model&apos;s estimate at entry, not a promise and not
					the position&apos;s current chance of winning. The snapshot is
					intentionally frozen, so this number never changes after entry.
				</p>
			</div>
		</Section.Body>
	</Section>
);

const FrozenFacts = ({ decision }: { decision: FrozenEntryDecision }) => (
	<div className="grid grid-cols-2 gap-2">
		<Section fit="content" surface="sunken">
			<Section.Header title="Entry economics" size="s" rule />
			<Section.Body>
				<Fact
					label="entry price"
					value={decision.entryCost.entryPrice}
					help="The volume-weighted price expected for the complete buy, not merely the best displayed ask."
				/>
				<Fact
					label="bid / ask / midpoint"
					value={`${display(decision.entryCost.bestBid)} / ${display(decision.entryCost.bestAsk)} / ${display(decision.entryCost.midpoint)}`}
					help="The visible market around the order. The gap between bid and ask is an immediate cost."
				/>
				<Fact
					label="spread / book impact"
					value={`${display(decision.entryCost.spread)} / ${display(decision.entryCost.impact)}`}
					help="Spread is the quoted gap; impact is the extra price paid while consuming available asks."
				/>
				<Fact
					label="round-trip fees"
					value={decision.entryCost.roundTripFees}
					help="Estimated buy and eventual sell fees combined."
				/>
				<Fact
					label="break-even sale price"
					value={decision.entryCost.breakEven}
					help="The sale price required merely to recover entry costs. Profit starts above this boundary."
				/>
			</Section.Body>
		</Section>

		<Section fit="content" surface="sunken">
			<Section.Header title="Risk and sizing" size="s" rule />
			<Section.Body>
				<Fact
					label="capital before entry"
					value={decision.availableCapital}
					help="Cash available when this order was sized. This is historical, not the account balance now."
				/>
				<Fact
					label="allocation"
					value={decision.allocationClass}
					help="The budget class that supplied this order after capacity checks."
				/>
			</Section.Body>
		</Section>
	</div>
);

export const EntryDecisionSnapshotView = ({
	decision,
}: {
	decision: FrozenEntryDecision;
}) => (
	<Flex.Column gap={3} className="p-4">
		<div className="rounded-[4px] border border-[color-mix(in_srgb,var(--acc)_35%,var(--line))] bg-[color-mix(in_srgb,var(--acc)_6%,var(--sunken))] px-4 py-3">
			<Flex.Row align="start" justify="between" gap={4}>
				<div>
					<Typography.Label size="xs" tone="accent">
						Why SYMM entered
					</Typography.Label>
					<p className="mt-1 text-[15px] text-(--f1) leading-snug">
						{display(decision.reason)}
					</p>
					<p className="mt-1 text-[10px] text-(--f4)">
						Its forecast direction was {directionText(decision.direction)} in{" "}
						{display(decision.cause)}.
					</p>
				</div>
				<div className="shrink-0 text-right font-mono text-[9px] text-(--f4)">
					<div className="text-(--acc)">FROZEN AT ENTRY</div>
					<div>{entryTime(decision.atNs)}</div>
					<div title={decision.id}>decision {decision.id.slice(0, 8)}</div>
				</div>
			</Flex.Row>
		</div>

		<DecisionPath decision={decision} />

		<div className="grid grid-cols-[minmax(0,1fr)_minmax(260px,0.55fr)] gap-2">
			<div className="min-w-0">
				<FrozenFacts decision={decision} />
			</div>
			<div className="min-w-0">
				<Confidence decision={decision} />
			</div>
		</div>

		<Section fit="content" surface="sunken">
			<Section.Header
				title="Evidence recorded on the decision"
				size="s"
				rule
				meta={`${decision.evidence.length} facts`}
			/>
			<Section.Body>
				<div className="grid grid-cols-2">
					{decision.evidence.map((entry) => (
						<div
							key={entry.key}
							className="border-(--line) border-r border-b px-3 py-2 last:border-b-0"
						>
							<Flex.Row align="baseline" justify="between" gap={3}>
								<span className="font-mono text-[9px] text-(--f2)">
									{entry.key}
								</span>
								<span className="text-right font-mono text-[9px] text-(--acc)">
									{evidenceValue(entry)}
								</span>
							</Flex.Row>
							<p className="mt-1 text-[9px] text-(--f4) leading-relaxed">
								{evidenceMeaning(entry.key)}
							</p>
						</div>
					))}
				</div>
			</Section.Body>
		</Section>
	</Flex.Column>
);

export const EntryDecisionSnapshot = ({ symbol }: { symbol: string }) => {
	const decision = useSelector(
		positionStore,
		(state) => readEntryDecision(state, symbol),
		{ compare: (previous, next) => previous?.id === next?.id },
	);

	if (decision === null) {
		return (
			<div className="p-6 font-mono text-[10px] text-(--warn) leading-relaxed">
				The open position did not carry its entry decision. This is unavailable,
				not an empty or zero-valued snapshot.
			</div>
		);
	}

	return <EntryDecisionSnapshotView decision={decision} />;
};
