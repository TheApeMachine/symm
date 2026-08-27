import { useRef } from "react";
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

const Row = ({
	label,
	tone = "text-(--f1)",
}: {
	label: string;
	tone?: string;
}) => (
	<Flex.Row align="baseline" justify="between" className="gap-2">
		<Typography.Label size="xxs" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono
			size="s"
			data-row={label}
			className={cn("min-w-0 truncate text-right", tone)}
		>
			—
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

/*
readAlternative returns one named economic/causal value from a decision's
alternatives vector, or null when absent. Missing values stay missing rather
than collapsing to a fabricated zero.
*/
const readAlternative = (decision: Decision | null, name: string): number | null => {
	if (!decision) return null;

	for (let i = 0; i < decision.alternativesLength(); i++) {
		const entry = decision.alternatives(i, altObj);
		if (entry && entry.name() === name) return entry.value();
	}

	return null;
};

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => {
	const positionRef = useRef<HTMLDivElement>(null);
	const arbitrationRef = useRef<HTMLDivElement>(null);
	const causalRef = useRef<HTMLDivElement>(null);
	const cognitionRef = useRef<HTMLDivElement>(null);

	positionStore.subscribe((state) => {
		if (!positionRef.current) return;
		const last = state.getLast();
		if (!last) return;

		let targetPos: Position | null = null;
		let targetHolding: Holding | null = null;
		for (let i = 0; i < last.rowsLength(); i++) {
			const pos = last.rows(i, posObj);
			if (!pos) continue;
			const h = pos.holding(holdObj);
			if (h && h.symbol() === symbol) {
				targetPos = pos;
				targetHolding = h;
				break;
			}
		}

		const holding = targetHolding;
		const stoploss = holding?.stoploss(stopObj);


		const set = (q: string, value: string) => {
			const el = positionRef.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el) el.textContent = value;
		};

		set("status", holding?.status() ?? targetPos?.status() ?? "—");
		set("qty", holding ? fmt(holding.qty()) : "—");
		set("entry", holding ? fmt(holding.entryPrice()) : "—");
		set("mark", holding ? fmt(holding.mark()) : "—");
		set("pnl", holding ? fmt(holding.pnl()) : "—");
		set(
			"return",
			holding && Number.isFinite(holding.returnPct())
				? `${(holding.returnPct() * 100).toFixed(2)}%`
				: "—",
		);
		set("stop floor", stoploss ? fmt(stoploss.floor()) : "—");
		set("peak", stoploss ? fmt(stoploss.peak()) : "—");
		set("profit line", stoploss ? fmt(stoploss.profitLine()) : "—");
		set("stop status", stoploss?.status() ?? "—");
	});

	strategyStore.subscribe((state) => {
		if (!arbitrationRef.current) return;

		// Merge across the whole ring buffer: the clicked symbol may have been
		// absent from the latest delta round, so read its most recent decision
		// from any frame instead of only getLast().
		const frames = state.toArray();
		let liveDecision: Decision | null = null;

		for (let f = frames.length - 1; f >= 0 && !liveDecision; f--) {
			const frame = frames[f];

			for (let i = 0; i < frame.decisionsLength(); i++) {
				const dec = frame.decisions(i, decObj);
				if (dec && dec.symbol() === symbol) {
					liveDecision = dec;
					break;
				}
			}
		}

		const set = (q: string, value: string) => {
			const el = arbitrationRef.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el) el.textContent = value;
		};

		const sourceBadge = arbitrationRef.current.querySelector<HTMLElement>("[data-decision-source]");
		if (sourceBadge) {
			sourceBadge.textContent = "LIVE TICK";
		}

		set("action", liveDecision?.action() ?? "—");
		set("utility", liveDecision ? liveDecision.utility().toFixed(4) : "—");
		set("uncertainty", readAlternative(liveDecision, "economic:outcome_uncertainty")?.toFixed(4) ?? "—");
		set("visits", readAlternative(liveDecision, "economic:visits")?.toFixed(0) ?? "—");
		set("causal support", readAlternative(liveDecision, "causal:effective_support")?.toFixed(4) ?? "—");
		set("cause", liveDecision?.reason() ?? "—");
		set("reason", liveDecision?.reason() ?? "—");
		set("at", liveDecision?.at() ? new Date(Number(liveDecision.at())).toISOString().slice(11, 19) : "—");
		set("proposed notional", liveDecision ? fmt(liveDecision.proposedNotional()) : "—");
		set("proposed qty", liveDecision ? fmt(liveDecision.proposedQuantity()) : "—");
	});

	causalStore.subscribe((frames) => {
		if (!causalRef.current) return;
		const all = frames.toArray();

		let targetRow: Causal | null = null;
		for (let f = all.length - 1; f >= 0 && !targetRow; f--) {
			const frame = all[f];

			for (let i = 0; i < frame.rowsLength(); i++) {
				const row = frame.rows(i, causalObj);
				if (row && row.symbol() === symbol) {
					targetRow = row;
					break;
				}
			}
		}

		const set = (q: string, value: string) => {
			const el = causalRef.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el) el.textContent = value;
		};

		set("association", targetRow ? targetRow.association().toFixed(4) : "—");
		set("confidence", targetRow ? targetRow.confidence().toFixed(4) : "—");
		set("strength", targetRow ? targetRow.strength().toFixed(4) : "—");
	});

	cognitionStore.subscribe((state) => {
		if (!cognitionRef.current) return;
		const all = state.toArray();

		let targetRow: Cognition | null = null;
		for (let f = all.length - 1; f >= 0 && !targetRow; f--) {
			const frame = all[f];

			for (let i = 0; i < frame.rowsLength(); i++) {
				const row = frame.rows(i, cogObj);
				if (row && row.symbol() === symbol) {
					targetRow = row;
					break;
				}
			}
		}

		const set = (q: string, value: string) => {
			const el = cognitionRef.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el) el.textContent = value;
		};

		set("winner", targetRow?.winner() ?? "—");
		set("contrast", targetRow ? targetRow.contrast().toFixed(3) : "—");
		set("entropy", targetRow ? targetRow.entropyBits().toFixed(3) : "—");
		set("paths", targetRow ? String(targetRow.lookaheadPaths()) : "—");
	});

	return (
		<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
			<div ref={positionRef}>
				<Card title="Position">
					<Row label="status" />
					<Row label="qty" />
					<Row label="entry" />
					<Row label="mark" />
					<Row label="pnl" />
					<Row label="return" />
					<Row label="stop floor" />
					<Row label="peak" />
					<Row label="profit line" />
					<Row label="stop status" />
				</Card>
			</div>

			<div ref={arbitrationRef}>
				<Card
					title="Arbitration"
					badge={
						<span
							data-decision-source
							className="rounded border border-(--line2) bg-(--sunken) px-1.5 py-0.5 font-mono text-[8.5px] uppercase font-semibold text-(--f3)"
						>
							SNAPSHOT
						</span>
					}
				>
					<Row label="action" tone="text-(--acc) uppercase" />
					<Row label="utility" />
					<Row label="uncertainty" />
					<Row label="visits" />
					<Row label="causal support" />
					<Row label="cause" />
					<Row label="reason" />
					<Row label="at" />
					<Row label="proposed notional" />
					<Row label="proposed qty" />
				</Card>
			</div>

			<div ref={causalRef}>
				<Card title="Causal">
					<Row label="association" />
					<Row label="confidence" />
					<Row label="strength" />
				</Card>
			</div>

			<div ref={cognitionRef}>
				<Card title="Cognition">
					<Row label="winner" tone="text-(--acc)" />
					<Row label="contrast" />
					<Row label="entropy" />
					<Row label="paths" />
				</Card>
			</div>
		</div>
	);
};

