import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";
import { Callout } from "#/components/ui/callout";
import { DataRow } from "#/components/ui/data-row";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import { Section } from "#/components/ui/section";
import { StepCard } from "#/components/ui/step-card";
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
		<Grid cols={4} gap={2} responsive className="xl:grid-cols-4">
			<StepCard
				step={1}
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
			<StepCard
				step={2}
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
			<StepCard
				step={3}
				title="Expected move cleared costs"
				value={<span className="text-(--up)">{costGate}</span>}
				explanation="The forecast center had to exceed the fee-inclusive break-even return. If it had merely predicted an upward move without covering costs, cash would have won instead."
			/>
			<StepCard
				step={4}
				title="Risk-sized order admitted"
				value={`${display(decision.proposedNotional)} USD · ${display(decision.proposedQuantity)} units`}
				explanation="The final quantity was capped by visible liquidity, venue minimums, available cash, and the loss geometry recorded with this decision."
			/>
		</Grid>
	);
};

const Confidence = ({ decision }: { decision: FrozenEntryDecision }) => (
	<Section fit="content" surface="sunken">
		<Section.Header title="How convinced was the system?" size="s" rule />
		<Section.Body>
			<Flex.Column padding={3} gap={2}>
				<Flex.Row align="baseline" justify="between">
					<span className="font-mono text-[10px] text-(--f3)">
						probability of clearing entry costs
					</span>
					<span className="font-mono text-[15px] text-(--acc)">
						{confidenceText(decision.confidence)}
					</span>
				</Flex.Row>
				<Meter
					percent={decision.confidence * 100}
					variant="warning"
					layout="bar"
					size="m"
				/>
				<p className="text-[10px] text-(--f4) leading-relaxed">
					Read this as the model&apos;s estimate at entry, not a promise and not
					the position&apos;s current chance of winning. The snapshot is
					intentionally frozen, so this number never changes after entry.
				</p>
			</Flex.Column>
		</Section.Body>
	</Section>
);

const FrozenFacts = ({ decision }: { decision: FrozenEntryDecision }) => (
	<Grid cols={2} gap={2} responsive={false}>
		<Section fit="content" surface="sunken">
			<Section.Header title="Entry economics" size="s" rule />
			<Section.Body>
				<DataRow.Group border>
					<DataRow
						layout="detailed"
						label="entry price"
						value={decision.entryCost.entryPrice}
						help="The volume-weighted price expected for the complete buy, not merely the best displayed ask."
					/>
					<DataRow
						layout="detailed"
						label="bid / ask / midpoint"
						value={`${display(decision.entryCost.bestBid)} / ${display(decision.entryCost.bestAsk)} / ${display(decision.entryCost.midpoint)}`}
						help="The visible market around the order. The gap between bid and ask is an immediate cost."
					/>
					<DataRow
						layout="detailed"
						label="spread / book impact"
						value={`${display(decision.entryCost.spread)} / ${display(decision.entryCost.impact)}`}
						help="Spread is the quoted gap; impact is the extra price paid while consuming available asks."
					/>
					<DataRow
						layout="detailed"
						label="round-trip fees"
						value={decision.entryCost.roundTripFees}
						help="Estimated buy and eventual sell fees combined."
					/>
					<DataRow
						layout="detailed"
						label="break-even sale price"
						value={decision.entryCost.breakEven}
						help="The sale price required merely to recover entry costs. Profit starts above this boundary."
					/>
				</DataRow.Group>
			</Section.Body>
		</Section>

		<Section fit="content" surface="sunken">
			<Section.Header title="Risk and sizing" size="s" rule />
			<Section.Body>
				<DataRow.Group border>
					<DataRow
						layout="detailed"
						label="capital before entry"
						value={decision.availableCapital}
						help="Cash available when this order was sized. This is historical, not the account balance now."
					/>
					<DataRow
						layout="detailed"
						label="allocation"
						value={decision.allocationClass}
						help="The budget class that supplied this order after capacity checks."
					/>
				</DataRow.Group>
			</Section.Body>
		</Section>
	</Grid>
);

export const EntryDecisionSnapshotView = ({
	decision,
}: {
	decision: FrozenEntryDecision;
}) => (
	<Flex.Column gap={3} padding={4}>
		<Callout
			tone="accent"
			title="Why SYMM entered"
			meta={
				<>
					<div className="text-(--acc)">FROZEN AT ENTRY</div>
					<div>{entryTime(decision.atNs)}</div>
					<div title={decision.id}>decision {decision.id.slice(0, 8)}</div>
				</>
			}
		>
			<p className="mt-1 text-[15px] text-(--f1) leading-snug">
				{display(decision.reason)}
			</p>
			<p className="mt-1 text-[10px] text-(--f4)">
				Its forecast direction was {directionText(decision.direction)} in{" "}
				{display(decision.cause)}.
			</p>
		</Callout>

		<DecisionPath decision={decision} />

		<Grid.Sidebar
			sidebarWidth="minmax(260px, 0.55fr)"
			gap={2}
			className="grid-cols-[minmax(0,1fr)_minmax(260px,0.55fr)]"
		>
			<FrozenFacts decision={decision} />
			<Confidence decision={decision} />
		</Grid.Sidebar>

		<Section fit="content" surface="sunken">
			<Section.Header
				title="Evidence recorded on the decision"
				size="s"
				rule
				meta={`${decision.evidence.length} facts`}
			/>
			<Section.Body>
				<Grid cols={2} responsive={false}>
					{decision.evidence.map((entry) => (
						<DataRow
							key={entry.key}
							layout="detailed"
							label={entry.key}
							value={evidenceValue(entry)}
							help={evidenceMeaning(entry.key)}
							tone="accent"
							border="bottom"
							className="border-(--line) border-r border-b"
						/>
					))}
				</Grid>
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
			<Flex padding={6}>
				<Typography.Mono tone="f4" size="xs" className="text-(--warn) leading-relaxed">
					The open position did not carry its entry decision. This is
					unavailable, not an empty or zero-valued snapshot.
				</Typography.Mono>
			</Flex>
		);
	}

	return <EntryDecisionSnapshotView decision={decision} />;
};
