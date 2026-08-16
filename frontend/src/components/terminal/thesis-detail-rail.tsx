
import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";
import { Typography } from "@/components/ui/typography";
import { Panel } from "@/components/ui/panel";

/*
ThesisDetailRail is what the engine currently believes about one symbol.

Each panel is bound to the frame that owns its answer and scoped to the symbol
the modal was opened on: the arbitration to the strategy decisions, the readings
to the measurements batch, the position to the positions batch. The rail used to
be written from a snapshot assembled out of nine wire keys, five of which the
backend does not publish, so most of it could only ever show its empty state.
*/
const Row = ({
	label,
	bind,
	format,
	tone = "text-(--f1)",
}: {
	label: string;
	bind: string;
	format?: string;
	tone?: string;
}) => (
	<Flex.Row align="baseline" justify="between" className="gap-2">
		<Typography.Label size="xxs" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		{/*
			An empty string is a real answer here — a lot recovered from the venue
			has no cause or reason to give, and the arbitration that opened it was
			never seen by this process. It reads as a dash rather than as a gap.
		*/}
		<Typography.Mono
			size="s"
			data-paint={bind}
			data-paint-format={format}
			data-paint-empty="—"
			className={cn("min-w-0 truncate text-right", tone)}
		/>
	</Flex.Row>
);

const Card = ({
	title,
	children,
}: {
	title: string;
	children: React.ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<Typography.Label size="lg" tone="f1" className="mb-2 block">
			{title}
		</Typography.Label>
		<div className="flex flex-col gap-1">{children}</div>
	</Panel>
);

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => (
	<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
		<Component registerKey="strategy" select="decisions">
			{({ ref }) => (
				<div ref={ref} data-scope="symbol" data-filter={symbol}>
					<Card title="Arbitration">
						<Row label="action" bind="action" tone="text-(--acc) uppercase" />
						<Row label="direction" bind="direction" format="+.0f" />
						<Row label="thesis score" bind="thesisScore" format=".4f" />
						<Row label="confidence" bind="thesisConfidence" format=".1%" />
						<Row label="support" bind="thesisSupport" format=".4f" tone="text-(--up)" />
						<Row label="contradiction" bind="thesisContradiction" format=".4f" tone="text-(--down)" />
						<Row label="conditions" bind="thesisConditions" format=".4f" />
						<Row label="predictive" bind="predictiveStatus" />
						<Row label="task skill" bind="taskSkill" format=".3f" />
						<Row label="supported horizon" bind="forecastHorizon" />
						<Row label="opportunity" bind="opportunityType" />
						<Row label="reserve" bind="reserveReason" />
						<Row label="allocation" bind="allocationClass" />
						<Row label="graph path" bind="graphScore" format=".5f" />
						<Row label="entry VWAP" bind="entryCost.entryPrice" format=".8f" />
						<Row label="break-even" bind="entryCost.breakEven" format=".8f" />
						<Row label="round-trip fees" bind="entryCost.roundTripFees" format=".6f" />
						<Row label="spread" bind="entryCost.spread" format=".8f" />
						<Row label="impact" bind="entryCost.impact" format=".8f" />
						<Row label="notional" bind="proposedNotional" format=".2f" />
						<Row label="cause" bind="cause" />
						<Row label="reason" bind="reason" />
						<Row label="round" bind="arbitrationRound" />
						<Row label="at" bind="at" format="time" />
					</Card>
				</div>
			)}
		</Component>

		<Component registerKey="causal">
			{({ ref }) => (
				<div ref={ref} data-scope="symbol" data-filter={symbol}>
					<Card title="Causal">
						<Row label="association" bind="association" format=".4f" />
						<Row label="intervention" bind="doExpectation" format=".4f" />
						<Row label="confidence" bind="confidence" format=".4f" />
						<Row label="entry line" bind="entry_baseline" format=".6f" />
						<Row label="strength" bind="strength" format=".4f" />
						<Row label="contagion" bind="contagion" format=".4f" />
					</Card>
				</div>
			)}
		</Component>

		<Component registerKey="cognition" select={symbol}>
			{({ ref }) => (
				<div ref={ref}>
					<Card title="Cognition">
						<Row label="winner" bind="winner" tone="text-(--acc)" />
						<Row label="confidence" bind="confidence" format=".1%" />
						<Row label="contrast" bind="contrast" format=".3f" />
						<Row label="entropy" bind="entropyBits" format=".3f" />
						<Row label="paths" bind="lookaheadPaths" />
					</Card>
				</div>
			)}
		</Component>

		{/*
			Why the lot exists, as opposed to what the planner thinks now. The desk
			keeps the arbitration that opened each position verbatim, because by the
			time anyone asks, that round is long gone and the current decision batch
			has moved on to a different verdict for the same symbol.
		*/}
		<Component registerKey="positions">
			{({ ref }) => (
				<div ref={ref} data-scope="holding.symbol" data-filter={symbol}>
					<Card title="Entry decision">
						<Row
							label="action"
							bind="decision.action"
							tone="text-(--acc) uppercase"
						/>
						<Row label="cause" bind="decision.cause" />
						<Row label="reason" bind="decision.reason" />
						<Row label="direction" bind="decision.direction" format="+.0f" />
						<Row label="thesis score" bind="decision.thesisScore" format=".4f" />
						<Row label="confidence" bind="decision.thesisConfidence" format=".1%" />
						<Row label="support" bind="decision.thesisSupport" format=".4f" tone="text-(--up)" />
						<Row label="contradiction" bind="decision.thesisContradiction" format=".4f" tone="text-(--down)" />
						<Row label="conditions" bind="decision.thesisConditions" format=".4f" />
						<Row label="predictive" bind="decision.predictiveStatus" />
						<Row label="task skill" bind="decision.taskSkill" format=".3f" />
						<Row label="supported horizon" bind="decision.forecastHorizon" />
						<Row label="opportunity" bind="decision.opportunityType" />
						<Row label="reserve" bind="decision.reserveReason" />
						<Row label="allocation" bind="decision.allocationClass" />
						<Row label="graph path" bind="decision.graphScore" format=".5f" />
						<Row label="entry VWAP" bind="decision.entryCost.entryPrice" format=".8f" />
						<Row label="break-even" bind="decision.entryCost.breakEven" format=".8f" />
						<Row label="round-trip fees" bind="decision.entryCost.roundTripFees" format=".6f" />
						<Row label="spread" bind="decision.entryCost.spread" format=".8f" />
						<Row label="impact" bind="decision.entryCost.impact" format=".8f" />
						<Row label="notional" bind="decision.proposedNotional" format=".2f" />
						<Row label="quantity" bind="decision.proposedQuantity" format=".6f" />
						<Row label="risk distance" bind="decision.risk.risk_distance" format=".8f" />
						<Row label="decided" bind="decision.at" format="time" />
					</Card>
				</div>
			)}
		</Component>

		<Component registerKey="positions">
			{({ ref }) => (
				<div ref={ref} data-scope="holding.symbol" data-filter={symbol}>
					<Card title="Position">
						<Row label="status" bind="status" />
						<Row label="qty" bind="holding.qty" format=".6f" />
						<Row label="entry" bind="holding.entry_price" format=".6f" />
						<Row label="mark" bind="holding.mark" format=".6f" />
						<Row label="pnl" bind="holding.pnl" format=".4f" />
						<Row label="return %" bind="holding.return_pct" format=".2f" />
						<Row
							label="stop floor"
							bind="holding.stoploss.floor"
							format=".6f"
						/>
						<Row label="stop status" bind="holding.stoploss.status" />
					</Card>
				</div>
			)}
		</Component>
	</div>
);
