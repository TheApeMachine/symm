import { useSelector } from "@tanstack/react-store";
import { shallow } from "@tanstack/store";
import {
	causalStore,
	cognitionStore,
	positionStore,
	strategyStore,
} from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Causal } from "#/providers/telemetry/telemetry/causal";
import { Cognition } from "#/providers/telemetry/telemetry/cognition";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";
import { Position } from "#/providers/telemetry/telemetry/position";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

const fmt = (value: unknown, digits?: number): string => {
	if (typeof value === "number") {
		return digits !== undefined && Number.isFinite(value)
			? value.toFixed(digits)
			: String(value);
	}
	if (typeof value === "string") {
		return value !== "" ? value : "—";
	}
	return "—";
};

const num = (value: number | null | undefined, digits: number): string =>
	value === null || value === undefined || !Number.isFinite(value)
		? "—"
		: value.toFixed(digits);

const Row = ({
	label,
	value,
	tone = "text-(--f1)",
}: {
	label: string;
	value?: string | null;
	tone?: string;
}) => (
	<Flex.Row align="baseline" justify="between" className="gap-2">
		<Typography.Label size="xxs" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono
			size="s"
			className={cn("min-w-0 truncate text-right", tone)}
		>
			{value || "—"}
		</Typography.Mono>
	</Flex.Row>
);

const Card = ({
	title,
	badge,
	children,
}: {
	title: string;
	badge?: React.ReactNode;
	children: React.ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<div className="mb-2 flex items-center justify-between">
			<Typography.Label size="lg" tone="f1">
				{title}
			</Typography.Label>
			{badge}
		</div>
		<div className="flex flex-col gap-1">{children}</div>
	</Panel>
);

const posObj = new Position();
const holdObj = new Holding();
const stopObj = new Stoploss();
const decObj = new Decision();
const causalObj = new Causal();
const cogObj = new Cognition();
const altObj = new NamedNumber();

const readAlternative = (decision: Decision, name: string): number | null => {
	for (let i = 0; i < decision.alternativesLength(); i++) {
		const entry = decision.alternatives(i, altObj);
		if (entry && entry.name() === name) return entry.value();
	}

	return null;
};

/*
Each selector below merges across the whole ring buffer (newest frame first)
and returns plain display values — never flatbuffer view references — so React
renders them directly. The previous subscribe-in-body + querySelector approach
mutated the DOM after render, which is why the fields stayed on their "—"
placeholders.
*/

/*
scanLatest walks the ring buffer newest-frame-first and returns the first row
that matches the supplied predicate, projected through the supplied callback.
Each selector supplies its own row length, row access, matching, and projection
so a single traversal convention backs all four rails.
*/
const scanLatest = <TFrame, TRow, TResult>(
	frames: TFrame[],
	rowLength: (frame: TFrame) => number,
	rowAt: (frame: TFrame, index: number) => TRow | null,
	matches: (row: TRow) => boolean,
	project: (row: TRow) => TResult,
): TResult | null => {
	for (let frameIndex = frames.length - 1; frameIndex >= 0; frameIndex--) {
		const frame = frames[frameIndex];

		for (let rowIndex = 0; rowIndex < rowLength(frame); rowIndex++) {
			const row = rowAt(frame, rowIndex);

			if (row === null) {
				continue;
			}

			if (!matches(row)) {
				continue;
			}

			return project(row);
		}
	}

	return null;
};

const selectPosition = (
	state: ReturnType<typeof positionStore.get>,
	symbol: string,
) =>
	scanLatest(
		state.toArray(),
		(frame) => frame.rowsLength(),
		(frame, index) => frame.rows(index, posObj),
		(pos) => {
			const holding = pos.holding(holdObj);
			return holding !== null && holding.symbol() === symbol;
		},
		(pos) => {
			const holding = pos.holding(holdObj)!;
			const stoploss = holding.stoploss(stopObj);

			return {
				status: holding.status() ?? pos.status() ?? "",
				qty: fmt(holding.qty()),
				entry: fmt(holding.entryPrice()),
				mark: fmt(holding.mark()),
				pnl: fmt(holding.pnl()),
				returnPct: Number.isFinite(holding.returnPct())
					? `${(holding.returnPct() * 100).toFixed(2)}%`
					: "—",
				stopFloor: fmt(stoploss?.floor()),
				peak: fmt(stoploss?.peak()),
				profitLine: fmt(stoploss?.profitLine()),
				stopStatus: stoploss?.status() ?? "",
			};
		},
	);

const selectDecision = (
	state: ReturnType<typeof strategyStore.get>,
	symbol: string,
) =>
	scanLatest(
		state.toArray(),
		(frame) => frame.decisionsLength(),
		(frame, index) => frame.decisions(index, decObj),
		(dec) => dec.symbol() === symbol,
		(dec) => {
			const utilityAvailable = dec.utilityAvailable();

			return {
				action: dec.action() ?? "",
				utility: utilityAvailable ? num(dec.utility(), 4) : "—",
				uncertainty: num(readAlternative(dec, "economic:outcome_uncertainty"), 4),
				visits: num(readAlternative(dec, "economic:visits"), 0),
				causalSupport: num(readAlternative(dec, "causal:effective_support"), 4),
				cause: dec.cause() ?? "",
				reason: dec.reason() ?? "",
				at: dec.at()
					? new Date(Number(dec.at() / 1000000n)).toISOString().slice(11, 19)
					: "",
				proposedNotional: fmt(dec.proposedNotional()),
				proposedQuantity: fmt(dec.proposedQuantity()),
			};
		},
	);

const selectCausal = (
	state: ReturnType<typeof causalStore.get>,
	symbol: string,
) =>
	scanLatest(
		state.toArray(),
		(frame) => frame.rowsLength(),
		(frame, index) => frame.rows(index, causalObj),
		(row) => row.symbol() === symbol,
		(row) => ({
			association: num(row.association(), 4),
			confidence: num(row.confidence(), 4),
			strength: num(row.strength(), 4),
		}),
	);

const selectCognition = (
	state: ReturnType<typeof cognitionStore.get>,
	symbol: string,
) =>
	scanLatest(
		state.toArray(),
		(frame) => frame.rowsLength(),
		(frame, index) => frame.rows(index, cogObj),
		(row) => row.symbol() === symbol,
		(row) => ({
			winner: row.winner() ?? "",
			contrast: num(row.contrast(), 3),
			entropy: num(row.entropyBits(), 3),
			paths: String(row.lookaheadPaths()),
		}),
	);

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => {
	const position = useSelector(
		positionStore,
		(state) => selectPosition(state, symbol),
		{ compare: shallow },
	);
	const decision = useSelector(
		strategyStore,
		(state) => selectDecision(state, symbol),
		{ compare: shallow },
	);
	const causal = useSelector(
		causalStore,
		(state) => selectCausal(state, symbol),
		{ compare: shallow },
	);
	const cognition = useSelector(
		cognitionStore,
		(state) => selectCognition(state, symbol),
		{ compare: shallow },
	);

	return (
		<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
			<Card title="Position">
				<Row label="status" value={position?.status} />
				<Row label="qty" value={position?.qty} />
				<Row label="entry" value={position?.entry} />
				<Row label="mark" value={position?.mark} />
				<Row label="pnl" value={position?.pnl} />
				<Row label="return" value={position?.returnPct} />
				<Row label="stop floor" value={position?.stopFloor} />
				<Row label="peak" value={position?.peak} />
				<Row label="profit line" value={position?.profitLine} />
				<Row label="stop status" value={position?.stopStatus} />
			</Card>

			<Card
				title="Arbitration"
				badge={
					<span
						data-decision-source
						className="rounded border border-(--line2) bg-(--sunken) px-1.5 py-0.5 font-mono text-[8.5px] uppercase font-semibold text-(--f3)"
					>
						{decision ? "LIVE TICK" : "NO DATA"}
					</span>
				}
			>
				<Row label="action" value={decision?.action} tone="text-(--acc) uppercase" />
				<Row label="utility" value={decision?.utility} />
				<Row label="uncertainty" value={decision?.uncertainty} />
				<Row label="visits" value={decision?.visits} />
				<Row label="causal support" value={decision?.causalSupport} />
				<Row label="cause" value={decision?.cause} />
				<Row label="reason" value={decision?.reason} />
				<Row label="at" value={decision?.at} />
				<Row label="proposed notional" value={decision?.proposedNotional} />
				<Row label="proposed qty" value={decision?.proposedQuantity} />
			</Card>

			<Card title="Causal">
				<Row label="association" value={causal?.association} />
				<Row label="confidence" value={causal?.confidence} />
				<Row label="strength" value={causal?.strength} />
			</Card>

			<Card title="Cognition">
				<Row label="winner" value={cognition?.winner} tone="text-(--acc)" />
				<Row label="contrast" value={cognition?.contrast} />
				<Row label="entropy" value={cognition?.entropy} />
				<Row label="paths" value={cognition?.paths} />
			</Card>
		</div>
	);
};
