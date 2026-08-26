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
		const last = state.getLast();
		if (!last) return;

		let liveDecision: Decision | null = null;
		for (let i = 0; i < last.decisionsLength(); i++) {
			const dec = last.decisions(i, decObj);
			if (dec && dec.symbol() === symbol) {
				liveDecision = dec;
				break;
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
		set("thesis score", liveDecision ? liveDecision.thesisScore().toFixed(4) : "—");
		set("confidence", liveDecision ? `${(liveDecision.confidence() * 100).toFixed(1)}%` : "—");
		set("graph score", liveDecision ? liveDecision.graphScore().toFixed(4) : "—");
		set("utility", liveDecision ? liveDecision.utility().toFixed(4) : "—");
		set("margin", liveDecision ? liveDecision.opportunityMargin().toFixed(4) : "—");
		set("cause", liveDecision?.reason() ?? "—");
		set("reason", liveDecision?.reason() ?? "—");
		set("at", liveDecision?.at() ? new Date(Number(liveDecision.at())).toISOString().slice(11, 19) : "—");
		set("proposed notional", liveDecision ? fmt(liveDecision.proposedNotional()) : "—");
		set("proposed qty", liveDecision ? fmt(liveDecision.proposedQuantity()) : "—");
	});

	causalStore.subscribe((frames) => {
		if (!causalRef.current) return;
		const lastFrame = frames.getLast();
		if (!lastFrame) return;

		let targetRow: Causal | null = null;
		for (let i = 0; i < lastFrame.rowsLength(); i++) {
			const row = lastFrame.rows(i, causalObj);
			if (row && row.symbol() === symbol) {
				targetRow = row;
				break;
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
		const last = state.getLast();
		if (!last) return;

		let targetRow: Cognition | null = null;
		for (let i = 0; i < last.rowsLength(); i++) {
			const row = last.rows(i, cogObj);
			if (row && row.symbol() === symbol) {
				targetRow = row;
				break;
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
					<Row label="thesis score" />
					<Row label="confidence" />
					<Row label="graph score" />
					<Row label="utility" />
					<Row label="margin" />
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

