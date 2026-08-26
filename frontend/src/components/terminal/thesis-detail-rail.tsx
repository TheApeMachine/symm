import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";
import { Typography } from "@/components/ui/typography";
import { Panel } from "@/components/ui/panel";
import {
	causalStore,
	cognitionStore,
	positionsStore,
	strategyStore,
	useSubscribe,
} from "#/providers/ws-stores";

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

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => {
	const positionRef = useSubscribe(positionsStore, (state) => {
		const row = state.positions[symbol]?.latest() ?? null;
		const holding = row?.holding;
		const stoploss = holding?.stoploss;

		const set = (q: string, value: string) => {
			const el = positionRef.current?.querySelector<HTMLElement>(
				`[data-row="${q}"]`,
			);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("status", holding?.status ?? row?.status ?? "—");
		set("qty", fmt(holding?.qty));
		set("entry", fmt(holding?.entry_price));
		set("mark", fmt(holding?.mark));
		set("pnl", fmt(holding?.pnl));
		set(
			"return",
			holding?.return_pct !== undefined && Number.isFinite(holding.return_pct)
				? `${(holding.return_pct * 100).toFixed(2)}%`
				: "—",
		);
		set("stop floor", fmt(stoploss?.floor));
		set("peak", fmt(stoploss?.peak));
		set("profit line", fmt(stoploss?.profit_line));
		set("stop status", stoploss?.status ?? "—");
	}, [symbol]);

	const arbitrationRef = useSubscribe(
		[positionsStore, strategyStore],
		([pState, sState]) => {
			const position = pState.positions[symbol]?.latest() ?? null;
			const liveDecision =
				(sState?.decisions ?? []).find((entry) => entry.symbol === symbol) ??
				null;

			// Prefer the originating entry decision snapshot that created this position,
			// or fallback to the live strategy decision.
			const decision = position?.decision ?? liveDecision;

			const set = (q: string, value: string) => {
				const el = arbitrationRef.current?.querySelector<HTMLElement>(
					`[data-row="${q}"]`,
				);
				if (el instanceof HTMLElement) el.textContent = value;
			};

			const isOriginating = Boolean(
				position?.decision && position.decision.id !== "",
			);
			const sourceBadge = arbitrationRef.current?.querySelector<HTMLElement>(
				"[data-decision-source]",
			);
			if (sourceBadge instanceof HTMLElement) {
				sourceBadge.textContent = isOriginating ? "ENTRY SNAPSHOT" : "LIVE TICK";
			}

			set("action", decision?.action ?? "—");
			set(
				"thesis score",
				decision?.thesisScore !== undefined
					? decision.thesisScore.toFixed(4)
					: "—",
			);
			set(
				"confidence",
				decision?.thesisConfidence !== undefined
					? `${(decision.thesisConfidence * 100).toFixed(1)}%`
					: "—",
			);
			set(
				"graph score",
				decision?.graphScore !== undefined
					? decision.graphScore.toFixed(4)
					: "—",
			);
			set(
				"utility",
				decision?.utility !== undefined ? decision.utility.toFixed(4) : "—",
			);
			set(
				"margin",
				decision?.opportunityMargin !== undefined
					? decision.opportunityMargin.toFixed(4)
					: "—",
			);
			set("cause", decision?.cause ?? "—");
			set("reason", decision?.reason ?? "—");
			set(
				"at",
				decision?.at
					? new Date(decision.at).toISOString().slice(11, 19)
					: "—",
			);
			set("proposed notional", fmt(decision?.proposedNotional));
			set("proposed qty", fmt(decision?.proposedQuantity));
		},
		[symbol],
	);

	const causalRef = useSubscribe(causalStore, (frames) => {
		const row = (frames ?? []).find((frame) => frame.symbol === symbol) ?? null;
		const set = (q: string, value: string) => {
			const el = causalRef.current?.querySelector<HTMLElement>(
				`[data-row="${q}"]`,
			);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set(
			"association",
			row?.association === undefined ? "—" : row.association.toFixed(4),
		);
		set(
			"confidence",
			row?.confidence === undefined ? "—" : row.confidence.toFixed(4),
		);
		set(
			"strength",
			row?.strength === undefined ? "—" : row.strength.toFixed(4),
		);
	}, [symbol]);

	const cognitionRef = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[symbol]?.latest() ?? null;
		const set = (q: string, value: string) => {
			const el = cognitionRef.current?.querySelector<HTMLElement>(
				`[data-row="${q}"]`,
			);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("winner", row?.winner === undefined ? "—" : String(row.winner));
		set(
			"contrast",
			row?.contrast === undefined ? "—" : row.contrast.toFixed(3),
		);
		set(
			"entropy",
			row?.entropyBits === undefined ? "—" : row.entropyBits.toFixed(3),
		);
		set(
			"paths",
			row?.lookaheadPaths === undefined ? "—" : String(row.lookaheadPaths),
		);
	}, [symbol]);

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
