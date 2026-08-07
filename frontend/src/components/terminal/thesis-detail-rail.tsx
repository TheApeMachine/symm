import { Component } from "#/components/ui/component";
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
	<div className="flex items-baseline justify-between gap-2 font-mono text-[10px]">
		<span className="text-(--f4)">{label}</span>
		<span data-paint={bind} data-paint-format={format} className={tone} />
	</div>
);

const Card = ({
	title,
	children,
}: {
	title: string;
	children: React.ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<div className="mb-2 font-semibold text-[11px] text-(--f1) uppercase tracking-[0.08em]">
			{title}
		</div>
		<div className="flex flex-col gap-1">{children}</div>
	</Panel>
);

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => (
	<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
		<Component registerKey="strategy" select="decisions">
			{({ ref }) => (
				<div ref={ref} data-scope="symbol" data-filter={symbol}>
					<Card title="Arbitration">
						<Row
							label="action"
							bind="action"
							tone="text-(--acc) uppercase"
						/>
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
						<Row label="stop floor" bind="holding.stoploss.floor" format=".6f" />
						<Row label="stop status" bind="holding.stoploss.status" />
					</Card>
				</div>
			)}
		</Component>
	</div>
);
