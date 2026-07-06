import { useSelector } from "@tanstack/react-store";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { positionsStore } from "#/collections/positions";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed, whyLabel } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";

type Frame = Record<string, unknown>;

const records = (value: unknown): Frame[] =>
	Array.isArray(value)
		? value.flatMap((item) =>
				item !== null && typeof item === "object" && !Array.isArray(item)
					? [item as Frame]
					: [],
			)
		: [];

const stringField = (frame: Frame, keys: string[], fallback = "-"): string => {
	for (const key of keys) {
		const value = frame[key];

		if (typeof value === "string" && value.trim() !== "") {
			return value;
		}
	}

	return fallback;
};

const numberField = (frame: Frame, keys: string[]): number | null => {
	for (const key of keys) {
		const value = Number(frame[key]);

		if (Number.isFinite(value)) {
			return value;
		}
	}

	return null;
};

const scoreText = (frame: Frame, keys: string[]): string => {
	const value = numberField(frame, keys);

	return value === null ? "-" : fixed(value);
};

const percentText = (value: number | null): string => {
	if (value === null) {
		return "-";
	}

	const percent = Math.abs(value) <= 1 ? value * 100 : value;

	return `${percent.toFixed(2)}%`;
};

const moneyText = (value: number | null, quote = ""): string => {
	if (value === null) {
		return "-";
	}

	const suffix = quote === "" ? "" : ` ${quote}`;

	return `${value.toFixed(4)}${suffix}`;
};

const decisionSymbol = (decision: Frame): string =>
	stringField(decision, ["symbol", "scope"]);

const decisionEdge = (decision: Frame): string => {
	const edge = numberField(decision, ["edge", "edge_pct", "net_edge"]);
	const required = numberField(decision, [
		"required_edge",
		"requiredEdge",
		"fee_pct",
		"cost",
	]);

	if (edge !== null && required !== null) {
		return `${percentText(edge)} / ${percentText(required)}`;
	}

	if (edge !== null || required !== null) {
		return percentText(edge ?? required);
	}

	return whyLabel(String(decision.why ?? decision.reason ?? ""));
};

const verdictClass = (verdict: string): string =>
	cn(
		"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
		verdict === "allow" &&
			"bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)",
		(verdict === "deny" || verdict === "blocked" || verdict === "reject") &&
			"bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)",
		verdict !== "allow" &&
			verdict !== "deny" &&
			verdict !== "blocked" &&
			verdict !== "reject" &&
			"bg-(--line) text-(--f3)",
	);

const DecisionRows = ({ decisions }: { decisions: Frame[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_58px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
			<span>Symbol</span>
			<span className="text-right">Comb</span>
			<span className="text-right">Edge</span>
			<span className="text-right">Verdict</span>
		</div>
		{decisions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for decision frames
			</div>
		) : null}
		{decisions.map((decision) => {
			const verdict = String(decision.verdict ?? "-").toLowerCase();

			return (
				<div
					key={String(
						decision.id ?? `${decisionSymbol(decision)}:${decision.tick}`,
					)}
					data-symbol={decisionSymbol(decision)}
					className="grid grid-cols-[78px_58px_minmax(84px,1fr)_58px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[11px]"
				>
					<div className="min-w-0">
						<div className="truncate font-semibold text-(--f1)">
							{decisionSymbol(decision)}
						</div>
						<div className="truncate text-[9px] text-(--f4)">trader</div>
					</div>
					<span className="text-right text-(--f2)">
						{scoreText(decision, ["comb", "combined", "score"])}
					</span>
					<span className="truncate text-right text-(--down)">
						{decisionEdge(decision)}
					</span>
					<span className="text-right">
						<span className={verdictClass(verdict)}>{verdict}</span>
					</span>
				</div>
			);
		})}
	</div>
);

const PositionRows = ({
	positions,
	quote,
	observed,
}: {
	positions: Frame[];
	quote: string;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{positions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no open positions" : "waiting for position frames"}
			</div>
		) : null}
		{positions.slice(-8).map((position) => {
			const pnl = numberField(position, [
				"unrealizedPnl",
				"unrealized_pnl",
				"pnl",
				"profit",
			]);
			const pct = numberField(position, ["pnl_pct", "return_pct", "returnPct"]);

			return (
				<div
					key={stringField(position, ["symbol", "scope"])}
					className="border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
				>
					<div className="flex items-start justify-between gap-3">
						<span className="font-semibold text-(--f1)">
							{stringField(position, ["symbol", "scope"])}
						</span>
						<span className="text-right font-semibold text-(--down)">
							P/L {moneyText(pnl, quote)}
						</span>
					</div>
					<div className="mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)">
						<span>
							entry{" "}
							{scoreText(position, ["entry", "entry_price", "entryPrice"])} /
							mark {scoreText(position, ["mark", "mark_price", "markPrice"])}
						</span>
						<span className="text-(--down)">{percentText(pct)}</span>
					</div>
				</div>
			);
		})}
	</div>
);

const auditTitle = (entry: Frame): string => {
	if (entry.role === "diagnostic") {
		return whyLabel(String(entry.reason ?? entry.why ?? ""));
	}

	return whyLabel(
		String(entry.order_status ?? entry.status ?? entry.role ?? ""),
	);
};

const auditDetail = (entry: Frame): string => {
	const symbol = stringField(entry, ["symbol", "scope"], "");
	const source = stringField(entry, ["source"], "");

	return [
		String(entry.verdict ?? entry.action ?? entry.role ?? ""),
		symbol,
		source,
	]
		.filter((part) => part !== "")
		.join(" / ");
};

const auditStamp = (entry: Frame): string =>
	String(
		entry.tick ?? entry.seq ?? entry.timestamp ?? entry.observed_at ?? "-",
	);

const AuditRows = ({ entries }: { entries: Frame[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{entries.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for diagnostics or execution frames
			</div>
		) : null}
		{entries.map((entry) => (
			<div
				key={String(entry.uuid ?? `${entry.role}:${auditStamp(entry)}`)}
				className="border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
			>
				<div className="flex items-center justify-between gap-3">
					<span className="font-semibold text-(--f2)">{auditTitle(entry)}</span>
					<span className="text-[9px] text-(--f4)">#{auditStamp(entry)}</span>
				</div>
				<div className="mt-1 truncate text-[10px] text-(--f4)">
					{auditDetail(entry)}
				</div>
			</div>
		))}
	</div>
);

export const DashboardRail = () => {
	const decision = useSelector(decisionStore, (state) => state);
	const positionsFrame = useSelector(positionsStore, (state) => state.frame);
	const diagnostics = useSelector(diagnosticsStore, (state) => state.history);
	const executions = useSelector(executionsStore, (state) => state.history);
	const positions = records(positionsFrame?.positions);
	const quote = stringField(
		positionsFrame ?? {},
		["quote", "quote_currency"],
		"USD",
	);
	const audit = [...diagnostics, ...executions].slice(-12).reverse();
	const net = numberField(positionsFrame ?? {}, [
		"net",
		"unrealizedPnl",
		"unrealized_pnl",
		"pnl",
	]);

	return (
		<div className="flex min-h-0 flex-col bg-(--surface)">
			<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
				<ColumnHeader
					title="Decisions"
					meta={
						<span>
							{decision.allowed.length} allow · {decision.denied.length} deny
						</span>
					}
				/>
				<DecisionRows
					decisions={decision.decisions.values().slice(-12).reverse()}
				/>
			</div>
			<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
				<ColumnHeader
					title="Open positions"
					meta={
						<span>
							{net === null ? "" : `net ${moneyText(net, quote)} · `}
							{positions.length} open
						</span>
					}
				/>
				<PositionRows
					positions={positions}
					quote={quote}
					observed={positionsFrame !== null}
				/>
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows entries={audit} />
			</div>
		</div>
	);
};
