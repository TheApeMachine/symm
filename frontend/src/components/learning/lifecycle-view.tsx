import { useSelector } from "@tanstack/react-store";
import {
	Activity,
	ArrowRight,
	CheckCircle2,
	ChevronDown,
	ChevronRight,
	Compass,
	Flag,
	Layers,
	Lock,
	Scale,
	ShieldAlert,
} from "lucide-react";
import { useState } from "react";
import { bookAtom } from "#/collections/book";
import { ExitPolicyPanel } from "#/components/learning/exit-policy-panel";
import { Badge, type BadgeVariant, badgeVariants } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	type Lifecycle,
	type LifeEvent,
	NOT_AVAILABLE,
	ratePercent,
	seconds,
	selectBook,
	useLifecycles,
} from "#/lib/lifecycle";
import { stateTone } from "#/lib/stop-telemetry";
import { formatExitTrigger } from "#/lib/trigger-labels";
import { cn } from "#/lib/utils";

/*
Confluence region taxonomy mapped to human-readable names and visual color tones.
*/
const REGION_INFO: Record<
	string,
	{ name: string; tone: "up" | "down" | "acc" | "info" | "neutral" | "warn" }
> = {
	R00: { name: "Enter detector", tone: "up" },
	R01: { name: "Enter support", tone: "up" },
	R02: { name: "Enter opposition", tone: "info" },
	R03: { name: "Exit detector", tone: "down" },
	R04: { name: "Exit support", tone: "down" },
	R05: { name: "Exit opposition", tone: "down" },
	R06: { name: "Hold detector", tone: "neutral" },
	R07: { name: "Hold support", tone: "acc" },
	R08: { name: "Hold opposition", tone: "warn" },
	R09: { name: "Volume quote veto", tone: "warn" },
	R10: { name: "Volume quote approve", tone: "up" },
	R11: { name: "Day move veto", tone: "warn" },
	R12: { name: "Day move approve", tone: "up" },
	R13: { name: "Spread veto", tone: "warn" },
	R14: { name: "Spread approve", tone: "up" },
	R15: { name: "Book integrity veto", tone: "warn" },
	R16: { name: "Book integrity approve", tone: "up" },
	R17: { name: "Board confidence veto", tone: "warn" },
	R18: { name: "Board confidence approve", tone: "up" },
	R19: { name: "Contagion veto", tone: "warn" },
	R20: { name: "Contagion approve", tone: "up" },
	R21: { name: "Capacity veto", tone: "warn" },
	R22: { name: "Capacity approve", tone: "up" },
	R23: { name: "24h peak veto", tone: "warn" },
	R24: { name: "24h peak approve", tone: "up" },
};

const TONE_STYLES = {
	up: "bg-(--up)/15 border-(--up)/30 text-(--up)",
	down: "bg-(--down)/15 border-(--down)/30 text-(--down)",
	acc: "bg-(--acc)/15 border-(--acc)/30 text-(--acc)",
	info: "bg-(--info)/15 border-(--info)/30 text-(--info)",
	neutral: "bg-(--surface) border-(--line) text-(--f2)",
	warn: "bg-(--warn)/15 border-(--warn)/30 text-(--warn)",
};

/*
PathChain turns raw prefix strings ("R02/R12/R02") into visual confluence token tiles.
*/
const PathChain = ({ path }: { path: string }) => {
	const tokens = path.split("/").filter(Boolean);
	if (tokens.length === 0) return <span>{path}</span>;

	return (
		<Flex.Row align="center" gap={1} wrap="wrap">
			{tokens.map((tok, i) => {
				const info = REGION_INFO[tok];
				const style = info ? TONE_STYLES[info.tone] : TONE_STYLES.neutral;
				return (
					// biome-ignore lint/suspicious/noArrayIndexKey: tokens in path sequence have deterministic position order
					<Flex.Row key={`${tok}-${i}`} align="center" gap={1}>
						{i > 0 && (
							<ArrowRight className="w-3 h-3 text-(--f4) shrink-0 select-none" />
						)}
						<Flex.Row
							align="center"
							gap={1}
							className={cn(
								badgeVariants({ size: "xs", variant: "disabled" }),
								"normal-case font-bold tracking-wide shadow-xs",
								style,
							)}
						>
							<span className="font-mono">{tok}</span>
							{info && (
								<span className="text-[9px] font-normal opacity-85 font-sans">
									{info.name}
								</span>
							)}
						</Flex.Row>
					</Flex.Row>
				);
			})}
		</Flex.Row>
	);
};

/*
KIND names each lifecycle event kind and its tone.
*/
const KIND: Record<string, { label: string; variant: BadgeVariant }> = {
	entry_match: { label: "ENTRY MATCH", variant: "brand" },
	sizing: { label: "SIZING", variant: "info" },
	child_order: { label: "CHILD", variant: "disabled" },
	order_submitted: { label: "ORDER", variant: "disabled" },
	order_failed: { label: "ORDER FAILED", variant: "error" },
	fill: { label: "FILL", variant: "info" },
	risk_sell: { label: "RISK", variant: "warning" },
	exit_match: { label: "EXIT MATCH", variant: "brand" },
	exit: { label: "EXIT", variant: "brand" },
	plan_ended: { label: "PLAN END", variant: "disabled" },
	closed: { label: "CLOSED", variant: "success" },
	abandoned: { label: "ABANDONED", variant: "disabled" },
};

const STATUS: Record<string, BadgeVariant> = {
	entering: "info",
	holding: "brand",
	exiting: "warning",
	closed: "success",
	abandoned: "disabled",
};

const trialVariant = (word: TrialWord): BadgeVariant => {
	switch (word) {
		case "win":
			return "success";
		case "loss":
			return "error";
		case "scratch":
			return "disabled";
		default: {
			const unreachable: never = word;
			return unreachable;
		}
	}
};

const time = (value?: string) =>
	value ? new Date(value).toLocaleTimeString(undefined, { hour12: false }) : "";

const fieldValue = (value: unknown): string => {
	if (typeof value === "number") {
		if (!Number.isFinite(value)) return String(value);
		const abs = Math.abs(value);
		if (abs !== 0 && (abs < 1e-3 || abs >= 1e7)) return value.toExponential(4);
		return Number(value.toPrecision(8)).toString();
	}
	if (typeof value === "string") {
		if (/^\d{4}-\d{2}-\d{2}T/.test(value)) {
			return value.startsWith("0001-") ? "—" : time(value);
		}
		return value;
	}
	if (typeof value === "boolean") return value ? "yes" : "no";
	if (value && typeof value === "object") {
		return Object.entries(value as Record<string, unknown>)
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([key, inner]) => `${key}:${fieldValue(inner)}`)
			.join(" ");
	}
	return "—";
};

const tone = (value: number) =>
	value > 0 ? "text-(--up)" : value < 0 ? "text-(--down)" : "text-(--f3)";

/*
SizingVisualWidget shows what the desk sized the entry on. Only the inputs the
sizing event carries are drawn: a constraint the desk did not report (book
exit depth, flow participation, a binding limit) has no bar, rather than a
bar at zero.
*/
const SIZING_FACTS: Array<[string, string, (value: number) => string]> = [
	["quantity", "Sized quantity", (value) => fieldValue(value)],
	["reference", "Ask at decision", (value) => `$${value.toFixed(2)}`],
	["entry_price", "Depth-walk entry price", (value) => `$${value.toFixed(2)}`],
	["total", "Entry cost incl. fee", (value) => `$${value.toFixed(2)}`],
	["entry_fee", "Entry fee", (value) => `$${value.toFixed(4)}`],
	["round_trip_fees", "Round-trip fees", (value) => `$${value.toFixed(4)}`],
	["break_even", "Break-even exit", (value) => `$${value.toFixed(2)}`],
	["spread", "Spread", (value) => fieldValue(value)],
	["impact", "Impact", (value) => fieldValue(value)],
];

const SizingVisualWidget = ({
	fields,
}: {
	fields: Record<string, unknown>;
}) => {
	const number = (key: string) =>
		typeof fields[key] === "number" ? (fields[key] as number) : undefined;
	const cash = number("cash");
	const allocated = number("allocated");
	const binding = typeof fields.binding === "string" ? fields.binding : null;
	const exitCap = number("exit_capacity");
	const partLimit = number("participation");
	const quantity = number("quantity");
	const facts = SIZING_FACTS.filter(([key]) => number(key) !== undefined);

	const cushion = (limit: number) =>
		quantity && quantity > 0 ? limit / quantity : undefined;

	return (
		<Flex.Column
			gap={2}
			data-l="lifecycle-sizing"
			className="p-3 bg-(--surface)/80 rounded border border-(--line) font-mono text-[11px]"
		>
			<Flex.Row
				align="center"
				justify="between"
				className="border-b border-(--line)/40 pb-1.5"
			>
				<Flex.Row align="center" gap={1}>
					<Scale className="w-3.5 h-3.5 text-(--info)" />
					<span className="font-bold text-(--f1) uppercase text-[10px]">
						Liquidity & Capital Boundaries
					</span>
				</Flex.Row>
				{binding ? (
					<Badge
						size="xxs"
						variant="warning"
						label={`Bound by ${binding.replace("_", " ")}`}
					/>
				) : null}
			</Flex.Row>

			<Grid cols={3} gap={2} responsive={false}>
				{cash !== undefined ? (
					<Flex.Column
						gap={1}
						className="p-2 rounded border bg-(--acc)/10 border-(--acc)/40"
					>
						<Flex.Row align="center" justify="between" className="text-[10px]">
							<Flex.Row align="center" gap={1} className="text-(--f4)">
								<Lock className="w-2.5 h-2.5 text-(--acc)" />
								<span>Cash allocated</span>
							</Flex.Row>
							<span className="font-bold text-(--acc)">
								{(allocated ?? cash).toFixed(2)} USD
							</span>
						</Flex.Row>
						<div className="w-full bg-(--line)/40 h-1.5 rounded-full overflow-hidden mt-0.5">
							<div
								className="h-full rounded-full bg-(--acc)"
								style={{
									width: `${Math.min(100, Math.max(0, ((allocated ?? cash) / Math.max(cash, 1e-12)) * 100))}%`,
								}}
							/>
						</div>
						<span className="text-[9px] text-(--f3) mt-0.5">
							of {cash.toFixed(2)} USD paper cash
						</span>
					</Flex.Column>
				) : null}

				{exitCap !== undefined ? (
					<Flex.Column
						gap={1}
						className="p-2 rounded border bg-(--sunken) border-(--line)"
					>
						<Flex.Row align="center" justify="between" className="text-[10px]">
							<span className="text-(--f4)">Book Exit Depth</span>
							<span className="font-bold text-(--up)">
								{exitCap.toFixed(2)} units
							</span>
						</Flex.Row>
						<span className="text-[9px] text-(--up) mt-0.5">
							{(cushion(exitCap) ?? 0) > 1
								? `${(cushion(exitCap) ?? 0).toFixed(0)}x Exit Cushion Available`
								: "Tight Exit Cushion"}
						</span>
					</Flex.Column>
				) : null}

				{partLimit !== undefined ? (
					<Flex.Column
						gap={1}
						className="p-2 rounded border bg-(--sunken) border-(--line)"
					>
						<Flex.Row align="center" justify="between" className="text-[10px]">
							<span className="text-(--f4)">Flow Participation</span>
							<span className="font-bold text-(--info)">
								{partLimit.toFixed(2)} units
							</span>
						</Flex.Row>
						<span className="text-[9px] text-(--info) mt-0.5">
							{(cushion(partLimit) ?? 0) > 1
								? `${(cushion(partLimit) ?? 0).toFixed(0)}x Impact Cushion`
								: "At Participation Cap"}
						</span>
					</Flex.Column>
				) : null}
			</Grid>

			{facts.length > 0 ? (
				<Grid
					cols={4}
					gap={2}
					responsive={false}
					className="text-[10px] pt-1 border-t border-(--line)/30"
				>
					{facts.map(([key, label, format]) => (
						<Flex.Column key={key}>
							<span className="text-(--f4) block text-[9px]">{label}:</span>
							<span className="font-bold text-(--f1)">
								{format(number(key) as number)}
							</span>
						</Flex.Column>
					))}
				</Grid>
			) : null}
		</Flex.Column>
	);
};

/*
FillVisualWidget is one paper fill: what filled at which price, and how much
of the order is filled so far.
*/
const FillVisualWidget = ({ fields }: { fields: Record<string, unknown> }) => {
	const qty = fieldValue(fields.quantity ?? 0);
	const price = Number(fields.price) || 0;
	const cost = Number(fields.cost) || 0;
	const fee = typeof fields.fee === "number" ? fields.fee : undefined;
	const filled = Number(fields.filled) || 0;
	const ordered = Number(fields.ordered) || 0;

	return (
		<Flex.Row
			align="center"
			justify="between"
			gap={3}
			wrap="wrap"
			className="p-2.5 bg-(--up)/5 rounded border border-(--up)/25 text-[11px] font-mono"
		>
			<Flex.Row align="center" gap={1}>
				<CheckCircle2 className="w-4 h-4 text-(--up) shrink-0" />
				<span className="font-bold text-(--up)">
					{fields.complete === true
						? "FILLED"
						: ordered > 0
							? `${((100 * filled) / ordered).toFixed(0)}% FILLED`
							: "PARTIAL"}
				</span>
				<span className="text-(--f1) font-bold ml-1">{qty} units</span>
			</Flex.Row>
			<Flex.Row align="center" gap={3} className="text-[10px]">
				<div>
					<span className="text-(--f4) mr-1">Paper price:</span>
					<span className="font-bold text-(--f1)">
						{price > 0 ? `$${price.toFixed(2)}` : "—"}
					</span>
				</div>
				<div>
					<span className="text-(--f4) mr-1">Cost:</span>
					<span className="font-bold text-(--f2)">
						{cost > 0 ? `$${cost.toFixed(2)}` : "—"}
					</span>
				</div>
				<div>
					<span className="text-(--f4) mr-1">Fee:</span>
					<span className="text-(--f3)">
						{fee !== undefined ? `$${fee.toFixed(4)}` : "—"}
					</span>
				</div>
				<div>
					<span className="text-(--f4) mr-1">Exchange fill:</span>
					<span className="text-(--f4)">{NOT_AVAILABLE}</span>
				</div>
			</Flex.Row>
		</Flex.Row>
	);
};

const EventRow = ({ event }: { event: LifeEvent }) => {
	const [expanded, setExpanded] = useState(false);
	const kind = KIND[event.kind] ?? {
		label: event.kind,
		variant: "disabled" as const,
	};
	const eventFields = event.fields ?? {};
	const fields = Object.entries(eventFields).sort(([a], [b]) =>
		a.localeCompare(b),
	);

	const icon = (() => {
		switch (event.kind) {
			case "entry_match":
				return <Compass className="w-3.5 h-3.5 text-(--acc) shrink-0" />;
			case "sizing":
				return <Scale className="w-3.5 h-3.5 text-(--info) shrink-0" />;
			case "order_submitted":
			case "child_order":
				return <Activity className="w-3.5 h-3.5 text-(--f3) shrink-0" />;
			case "fill":
				return <CheckCircle2 className="w-3.5 h-3.5 text-(--up) shrink-0" />;
			case "risk_sell":
				return <ShieldAlert className="w-3.5 h-3.5 text-(--warn) shrink-0" />;
			case "exit_match":
			case "exit":
				return <Flag className="w-3.5 h-3.5 text-(--acc) shrink-0" />;
			default:
				return <Layers className="w-3.5 h-3.5 text-(--f4) shrink-0" />;
		}
	})();

	return (
		<Flex.Column
			data-lifecycle-event={event.kind}
			className="border-(--line) border-b px-3 py-2 font-mono text-[11px] transition-colors hover:bg-(--surface)/40"
		>
			<Flex.Row align="center" gap={2} className="min-w-0">
				<span className="w-16 shrink-0 text-(--f4)" title={event.at}>
					{time(event.at)}
				</span>
				{icon}
				<Badge size="xs" variant={kind.variant} label={kind.label} />
				<span
					className="min-w-0 flex-1 truncate text-(--f1) font-medium"
					title={event.detail}
				>
					{event.detail}
				</span>
				{fields.length > 0 && (
					<button
						type="button"
						onClick={() => setExpanded(!expanded)}
						className="shrink-0 text-[9px] text-(--f4) hover:text-(--f2) border border-(--line) px-1.5 py-0.5 rounded bg-(--sunken)"
					>
						<Flex.Row align="center" gap={1}>
							<span>{fields.length} params</span>
							{expanded ? (
								<ChevronDown className="w-2.5 h-2.5" />
							) : (
								<ChevronRight className="w-2.5 h-2.5" />
							)}
						</Flex.Row>
					</button>
				)}
			</Flex.Row>

			{expanded && (
				<Flex.Column gap={2} className="mt-2 ml-6">
					{/* Specialized Visual Widgets for key events */}
					{event.kind === "entry_match" &&
						typeof eventFields.path === "string" && (
							<Flex.Column
								gap={1}
								className="p-2.5 bg-(--sunken) rounded border border-(--line)"
							>
								<span className="text-[10px] text-(--f4) block uppercase font-bold tracking-wider">
									Matched S3 Prefix Chain
								</span>
								<PathChain path={eventFields.path} />
							</Flex.Column>
						)}

					{event.kind === "sizing" && (
						<SizingVisualWidget fields={eventFields} />
					)}

					{event.kind === "fill" && <FillVisualWidget fields={eventFields} />}

					{/* Clean, Sorted Attribute Grid for all parameters */}
					<Grid.Auto
						minWidth="11rem"
						gapX={4}
						gapY={1}
						className="rounded bg-(--sunken)/80 border border-(--line)/40 p-2 text-[10px]"
					>
						{fields.map(([key, value]) => (
							<Flex.Row
								key={key}
								justify="between"
								align="center"
								gap={2}
								className="min-w-0"
							>
								<span className="text-(--f4)">{key}</span>
								<span
									className="truncate text-(--f2)"
									title={fieldValue(value)}
								>
									{fieldValue(value)}
								</span>
							</Flex.Row>
						))}
					</Grid.Auto>
				</Flex.Column>
			)}
		</Flex.Column>
	);
};

const shownLife = (life: Lifecycle): Lifecycle =>
	life.book === "virtual" && life.status === "open"
		? { ...life, status: "holding" }
		: life;

type TrialWord = "win" | "loss" | "scratch";

const trialWord = (life: Lifecycle): TrialWord | null => {
	if (life.book !== "virtual") return null;
	const word = life.outcome?.triggers?.[0];
	switch (word) {
		case "win":
		case "loss":
		case "scratch":
			return word;
		default:
			return null;
	}
};

const closedVerdict = (life: Lifecycle): TrialWord => {
	const word = trialWord(life);
	if (word) return word;
	return (life.outcome?.pnl ?? 0) > 0 ? "win" : "loss";
};

const pctMark = (value: number | null | undefined) =>
	value === null || value === undefined ? "—" : `${value.toFixed(2)}%`;

const moneyText = (value: number) =>
	`${value >= 0 ? "+$" : "−$"}${Math.abs(value).toFixed(2)}`;

/*
ClosedHero is the one place the settled result is stated: dollars first,
percent once, exit reason once, with the official exit mark beside it.
*/
const ClosedHero = ({ life }: { life: Lifecycle }) => {
	const outcome = life.outcome;
	if (!outcome || life.status !== "closed") return null;

	const verdict = closedVerdict(life);
	const money = outcome.money;
	const exitPct = life.official_exit_pct;
	const stop = life.stop_state;
	const stopState = stop?.state;
	const exitReason =
		life.exit_trigger && life.exit_trigger !== ""
			? formatExitTrigger(life.exit_trigger)
			: stop?.exit_reason
				? formatExitTrigger(stop.exit_reason)
				: null;
	const triggers =
		(outcome.triggers ?? [])
			.map(formatExitTrigger)
			.filter(Boolean)
			.join(" · ") ||
		(life.exit_trigger ? formatExitTrigger(life.exit_trigger) : "—");
	const path =
		life.path ||
		(typeof life.events?.find((e) => e.kind === "entry_match")?.fields?.path ===
		"string"
			? (life.events.find((e) => e.kind === "entry_match")?.fields
					?.path as string)
			: undefined);

	return (
		<Flex.Column
			data-l="lifecycle-outcome"
			className="border-b border-(--line) bg-(--sunken) font-mono"
		>
			<Flex.Row
				align="start"
				justify="between"
				gap={4}
				wrap="wrap"
				className="px-3 py-3"
			>
				<Flex.Column gap={1} className="min-w-0">
					<Flex.Row align="center" gap={2}>
						<Badge size="xs" variant={trialVariant(verdict)} label={verdict} />
						<span className="text-[10px] uppercase tracking-wider text-(--f4)">
							{life.book === "virtual" ? "virtual" : "paper"} settle
						</span>
					</Flex.Row>
					{money !== null && money !== undefined ? (
						<span
							className={cn(
								"text-[28px] font-bold tabular-nums leading-none",
								tone(money),
							)}
						>
							{moneyText(money)}
						</span>
					) : (
						<span
							className={cn(
								"text-[28px] font-bold tabular-nums leading-none",
								tone(outcome.pnl),
							)}
						>
							{ratePercent(outcome.pnl)}
						</span>
					)}
					<Flex.Row
						align="baseline"
						gap={2}
						className="text-[12px] text-(--f3)"
					>
						{money !== null && money !== undefined ? (
							<span className={cn("font-bold tabular-nums", tone(outcome.pnl))}>
								{ratePercent(outcome.pnl)}
							</span>
						) : null}
						<span>hold {seconds(outcome.hold_ns)}</span>
						{life.peak_pct !== null && life.peak_pct !== undefined ? (
							<span>peak {life.peak_pct.toFixed(2)}%</span>
						) : null}
					</Flex.Row>
					<span className="max-w-xl text-[11px] leading-snug text-(--f2) wrap-break-word">
						{triggers}
					</span>
					{path ? (
						<div className="pt-1">
							<PathChain path={path} />
						</div>
					) : null}
				</Flex.Column>

				<Flex.Column
					data-l="lifecycle-stop-exit"
					className="min-w-[16rem] flex-1 max-w-md rounded border border-(--line) bg-(--surface)"
				>
					<Flex.Row
						align="center"
						justify="between"
						className="h-7 px-3 border-b border-(--line) text-[10px] font-bold uppercase tracking-wider text-(--f3)"
					>
						<span>Exit</span>
						{stopState ? (
							<Badge
								size="xxs"
								variant={
									stateTone(stopState) === "bad"
										? "error"
										: stateTone(stopState) === "warn"
											? "warning"
											: stateTone(stopState) === "good"
												? "success"
												: "info"
								}
								label={stopState}
							/>
						) : (
							<Badge size="xxs" variant="disabled" label="no stop" />
						)}
					</Flex.Row>
					<Flex.Column gap={1} className="p-3 min-w-0">
						<span className="text-[9px] uppercase tracking-wider text-(--f4)">
							Official exit
						</span>
						<span
							className={cn(
								"text-[18px] font-bold tabular-nums",
								exitPct == null ? "text-(--f3)" : tone(exitPct),
							)}
						>
							{pctMark(exitPct)}
						</span>
						{exitReason ? (
							<span className="text-[10px] leading-snug text-(--f2) wrap-break-word">
								{exitReason}
							</span>
						) : null}
					</Flex.Column>
				</Flex.Column>
			</Flex.Row>
		</Flex.Column>
	);
};

/*
lifecycleRow projects a lifecycle back to the raw desk-row shape the stop
panels read.
*/
const lifecycleRow = (life: Lifecycle): Record<string, unknown> => {
	const outcome = life.outcome;
	const stop = life.stop_state;
	const current =
		outcome?.pnl_pct ??
		(life.mark != null && life.mark.unrealized_return != null
			? life.mark.unrealized_return * 100
			: undefined) ??
		stop?.current_pnl;

	return {
		id: life.id,
		symbol: life.symbol,
		book: life.book,
		opened_at: life.opened_at,
		closed_at: life.closed_at ?? null,
		entry_path: life.path,
		pnl_pct: typeof current === "number" ? current : undefined,
		exit_trigger: life.exit_trigger ?? undefined,
		outcome_triggers: outcome?.triggers ?? undefined,
		pnl: outcome?.money ?? undefined,
		stoploss: stop ?? undefined,
	};
};

const PathChart = ({ life }: { life: Lifecycle }) => {
	if (!life.pnl_trace || life.pnl_trace.length === 0) return null;
	return (
		<Flex.Column className="border-(--line) border-b bg-(--surface) px-3 py-2">
			<ExitPolicyPanel row={lifecycleRow(life)} />
		</Flex.Column>
	);
};

/*
LifecycleView lists settled round trips for the active book (closed desk lots
and archive rows). Open lots stay on Open Positions; exchange figures read
"not available", never zero.
*/
export const LifecycleView = () => {
	const { report } = useLifecycles();
	const book = useSelector(bookAtom, (state) => state);
	const view = selectBook(report, book);
	const [selected, setSelected] = useState<string | null>(null);
	const lifecycles = view.lifecycles
		.filter((life) => life.status === "closed")
		.map(shownLife)
		.sort((left, right) => {
			const leftAt = left.closed_at
				? Date.parse(left.closed_at)
				: Date.parse(left.opened_at);
			const rightAt = right.closed_at
				? Date.parse(right.closed_at)
				: Date.parse(right.opened_at);
			return rightAt - leftAt;
		});
	const perf = view.performance;
	const name = book === "virtual" ? "virtual" : "paper";
	const active =
		lifecycles.find((life) => life.id === selected) ?? lifecycles[0];

	return (
		<Flex.Column className="min-h-0 flex-1">
			<Flex.Row
				data-l="lifecycle-performance"
				align="center"
				gap={4}
				wrap="wrap"
				className="shrink-0 border-(--line) border-b bg-(--surface) px-3.5 py-1.5 font-mono text-[10px] text-(--f3) shadow-xs"
			>
				<div className="flex items-center gap-1.5">
					<span className="w-1.5 h-1.5 rounded-full bg-(--up) animate-pulse" />
					<span className="font-bold uppercase tracking-wider text-(--f4)">
						{name} outcome
					</span>
				</div>
				<Badge
					size="xs"
					variant="disabled"
					className="normal-case font-normal"
					label={`${perf.closed} closed`}
				/>
				<div className="flex items-center gap-2 rounded-xs bg-(--sunken) border border-(--line) px-2 py-0.5">
					<div className="w-12 h-1.5 rounded-full bg-(--bg) overflow-hidden border border-(--line) hidden sm:block">
						<div
							style={{
								width: `${Math.min(100, Math.max(0, (perf.win_rate ?? 0) * 100))}%`,
							}}
							className="h-full bg-(--info)"
						/>
					</div>
					<span>
						{name} win {ratePercent(perf.win_rate)} · edge{" "}
						{ratePercent(perf.mean_return)} · Σ return{" "}
						<span className={tone(perf.pnl)}>{ratePercent(perf.pnl)}</span>
					</span>
				</div>
				<Badge
					size="xs"
					variant="disabled"
					className="normal-case font-normal text-(--f4)"
					label={`exchange win · edge · P&L ${NOT_AVAILABLE}`}
				/>
			</Flex.Row>

			<Flex className="min-h-0 flex-1 max-lg:flex-col">
				<Flex.Column className="w-64 shrink-0 overflow-auto border-(--line) border-r bg-(--surface)/40 max-lg:w-full">
					{lifecycles.length === 0 ? (
						<Typography.Mono size="s" tone="f3" className="p-3">
							No closed positions yet. Open lots live under Open Positions;
							settled trips appear here when the desk closes a lot.
						</Typography.Mono>
					) : (
						lifecycles.map((life) => {
							const money = life.outcome?.money;
							const frac = life.outcome?.pnl;
							const isCurrent = active?.id === life.id;
							const word = trialWord(life);
							const variant =
								life.status === "closed" && word
									? trialVariant(word)
									: (STATUS[life.status] ?? "disabled");
							return (
								<button
									key={life.id}
									type="button"
									data-l="lifecycle-item"
									onClick={() => setSelected(life.id)}
									className={cn(
										"w-full cursor-pointer border-(--line) border-b px-2.5 py-2 text-left font-mono transition-all border-l-2",
										isCurrent
											? "bg-(--acc)/10 border-l-(--acc)"
											: "border-l-transparent hover:bg-(--sunken)",
									)}
								>
									<Flex.Row align="center" justify="between" gap={2}>
										<span className="truncate text-[11px] font-bold text-(--f1)">
											{life.symbol}
										</span>
										<Badge
											size="xxs"
											variant={variant}
											label={word ?? life.status}
										/>
									</Flex.Row>
									<Flex.Row
										justify="between"
										className="mt-0.5 text-[9px] text-(--f4)"
									>
										<span>{time(life.closed_at ?? life.opened_at)}</span>
										{money !== null && money !== undefined ? (
											<span className={cn("font-bold text-[9px]", tone(money))}>
												{moneyText(money)}
											</span>
										) : frac !== undefined ? (
											<span className={cn("font-bold text-[9px]", tone(frac))}>
												{ratePercent(frac)}
											</span>
										) : null}
									</Flex.Row>
								</button>
							);
						})
					)}
				</Flex.Column>

				<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-auto">
					{active ? (
						<>
							<Section.Header
								title={active.symbol}
								meta={`${time(active.opened_at)} → ${active.closed_at ? time(active.closed_at) : "—"}`}
							/>
							<ClosedHero life={active} />
							<PathChart life={active} />
							{(active.events ?? []).map((event, index) => (
								// biome-ignore lint/suspicious/noArrayIndexKey: events are append-only
								<EventRow key={index} event={event} />
							))}
						</>
					) : (
						<Typography.Mono size="s" tone="f3" className="p-3">
							No closed positions yet.
						</Typography.Mono>
					)}
				</Flex.Column>
			</Flex>
		</Flex.Column>
	);
};
