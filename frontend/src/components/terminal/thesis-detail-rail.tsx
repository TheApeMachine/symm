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
						<Row label="utility" bind="utility" format=".5f" />
						<Row label="confidence" bind="confidence" format=".1%" />
						<Row label="expected return" bind="expectedReturn" format=".6f" />
						<Row label="expected fees" bind="expectedFees" format=".6f" />
						<Row label="spread" bind="expectedSpread" format=".6f" />
						<Row label="impact" bind="expectedImpact" format=".6f" />
						<Row label="uncertainty" bind="uncertainty" format=".4f" />
						<Row
							label="haircut"
							bind="allocation_haircut"
							format=".1%"
							tone="text-(--down)"
						/>
						<Row label="notional" bind="proposedNotional" format=".2f" />
						<Row label="class" bind="allocationClass" />
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
						<Row label="utility" bind="decision.utility" format=".5f" />
						<Row label="confidence" bind="decision.confidence" format=".1%" />
						<Row
							label="expected return"
							bind="decision.expectedReturn"
							format=".6f"
						/>
						<Row
							label="expected fees"
							bind="decision.expectedFees"
							format=".6f"
						/>
						<Row label="spread" bind="decision.expectedSpread" format=".6f" />
						<Row label="impact" bind="decision.expectedImpact" format=".6f" />
						<Row
							label="adverse selection"
							bind="decision.adverseSelection"
							format=".6f"
						/>
						<Row label="uncertainty" bind="decision.uncertainty" format=".4f" />
						<Row
							label="opportunity margin"
							bind="decision.opportunityMargin"
							format=".4f"
						/>
						<Row
							label="cognitive lead"
							bind="decision.cognitiveLead"
							format=".4f"
						/>
						<Row
							label="basin confidence"
							bind="decision.basinConfidence"
							format=".4f"
						/>
						<Row
							label="haircut"
							bind="decision.allocation_haircut"
							format=".1%"
							tone="text-(--down)"
						/>
						<Row
							label="haircut reason"
							bind="decision.allocation_haircut_reason"
						/>
						<Row label="class" bind="decision.allocationClass" />
						<Row
							label="notional"
							bind="decision.proposedNotional"
							format=".2f"
						/>
						<Row
							label="quantity"
							bind="decision.proposedQuantity"
							format=".6f"
						/>
						<Row
							label="reference"
							bind="decision.referencePrice"
							format=".6f"
						/>
						<Row label="forecast model" bind="decision.forecastModel" />
						<Row label="round" bind="decision.arbitrationRound" />
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
						<Row label="return" bind="holding.return_pct" format=".2%" />
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
