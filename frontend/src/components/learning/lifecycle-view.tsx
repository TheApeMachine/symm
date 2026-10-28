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
	Radio,
	Scale,
	ShieldAlert,
	ShieldCheck,
	Zap,
} from "lucide-react";
import { useState } from "react";
import { bookAtom } from "#/collections/book";
import {
	Badge,
	BadgeButton,
	type BadgeVariant,
	badgeVariants,
} from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { hubBaseUrl } from "#/lib/hub";
import {
	type Lifecycle,
	type LifeEvent,
	NOT_AVAILABLE,
	ratePercent,
	seconds,
	selectBook,
	signedMoney,
	useLifecycles,
} from "#/lib/lifecycle";
import { formatExitTrigger } from "#/lib/trigger-labels";
import { cn } from "#/lib/utils";

/*
Confluence region taxonomy mapped to human-readable names and visual color tones.
*/
const REGION_INFO: Record<
	string,
	{ name: string; tone: "up" | "down" | "acc" | "info" | "neutral" | "warn" }
> = {
	R00: { name: "Enter Detector", tone: "up" },
	R01: { name: "Enter Support", tone: "up" },
	R02: { name: "Enter Opposition", tone: "info" },
	R03: { name: "Exit Detector", tone: "down" },
	R04: { name: "Exit Support", tone: "down" },
	R05: { name: "Exit Opposition", tone: "down" },
	R06: { name: "Coiled Compression", tone: "neutral" },
	R07: { name: "Explosive Ignition", tone: "acc" },
	R08: { name: "Turbulent Friction", tone: "warn" },
	R09: { name: "Dead / Starved", tone: "neutral" },
	R10: { name: "Macro Recurrence", tone: "up" },
	R11: { name: "Sentiment Recurrence", tone: "info" },
	R12: { name: "Friction Hurdle", tone: "warn" },
	R13: { name: "Macro Contagion", tone: "warn" },
	R14: { name: "Capacity / Liquidity", tone: "warn" },
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

const heldVirtual = (life: Lifecycle) =>
	(life.book ?? "paper") === "virtual" &&
	(life.status === "open" || life.status === "holding");

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

const LifecycleStepper = ({ life }: { life: Lifecycle }) => {
	const events = life.events ?? [];
	const hasMatch = events.some((e) => e.kind === "entry_match");
	const hasSizing = events.some((e) => e.kind === "sizing");
	const hasFill = events.some((e) => e.kind === "fill");
	const virtualHold = heldVirtual(life);
	const isHolding =
		life.status === "holding" ||
		life.status === "exiting" ||
		life.status === "closed" ||
		virtualHold;
	const hasExit = events.some(
		(e) =>
			e.kind === "exit" || e.kind === "exit_match" || e.kind === "risk_sell",
	);
	const isClosed = life.status === "closed";

	const stages = [
		{ label: "1. Match", done: hasMatch, active: !hasSizing && !virtualHold },
		{ label: "2. Sizing", done: hasSizing, active: hasSizing && !hasFill },
		{ label: "3. Fill", done: hasFill, active: hasFill && !isHolding },
		{
			label: "4. Hold",
			done: isHolding,
			active: life.status === "holding" || virtualHold,
		},
		{
			label: "5. Exit",
			done: hasExit || isClosed,
			active: life.status === "exiting",
		},
		{ label: "6. Outcome", done: isClosed, active: isClosed },
	];

	return (
		<Flex.Row
			align="center"
			gap={1}
			className="px-3 py-1.5 bg-(--surface) border-b border-(--line) overflow-x-auto text-[10px] font-mono shrink-0"
		>
			{stages.map((st, idx) => (
				<Flex.Row key={st.label} align="center" gap={1} className="shrink-0">
					{idx > 0 && <span className="text-(--f4) select-none">→</span>}
					<Badge
						size="xxs"
						variant={st.done ? "brand" : st.active ? "warning" : "disabled"}
						className={st.active ? "animate-pulse" : undefined}
						label={st.label}
					/>
				</Flex.Row>
			))}
		</Flex.Row>
	);
};

/*
StrategyIntelligenceCard:
Transforms the top summary into visual strategy recognition, liquidity constraints,
and real-time trie exit signal monitoring (replacing the bogus "Hold Horizon" timer).
*/
const StrategyIntelligenceCard = ({ life }: { life: Lifecycle }) => {
	const events = life.events ?? [];
	const entryMatch = events.find((e) => e.kind === "entry_match");
	const sizing = events.find((e) => e.kind === "sizing");
	const outcome = life.outcome;

	const entryPath = entryMatch?.fields?.path;
	const path =
		typeof entryPath === "string" && entryPath !== "" ? entryPath : life.path;
	const confidence = entryMatch?.fields?.confidence as number | undefined;
	const candidates = entryMatch?.fields?.candidates as number | undefined;

	const binding = sizing?.fields?.binding as string | undefined;
	const exitCapacity = sizing?.fields?.exit_capacity as number | undefined;
	const cash = (sizing?.fields?.allocated ?? sizing?.fields?.cash) as
		| number
		| undefined;
	const mark = life.mark;

	const isClosed = life.status === "closed";
	const isExiting = life.status === "exiting";
	const isHolding = life.status === "holding";

	return (
		<Grid
			cols={3}
			gap={0}
			responsive={false}
			className="border-b border-(--line) bg-(--surface) font-mono text-[11px]"
		>
			{/* 1. Strategy Pattern Visual Chain */}
			<Flex.Column className="border-r border-(--line) min-h-0 bg-(--surface)">
				<Flex.Row
					align="center"
					justify="between"
					className="h-7 px-3 bg-(--sunken) border-b border-(--line) text-[10px] text-(--f3) font-bold uppercase tracking-wider"
				>
					<Flex.Row align="center" gap={1}>
						<Compass className="w-3.5 h-3.5 text-(--acc)" />
						<span>Strategy Pattern</span>
					</Flex.Row>
					{confidence !== undefined && (
						<span className="text-(--up) font-bold">
							confidence {fieldValue(confidence)}
						</span>
					)}
				</Flex.Row>

				<Flex.Column className="p-3 flex-1 justify-between gap-2">
					<div>
						{path ? (
							<PathChain path={path} />
						) : (
							<span className="text-[11px] text-(--f3) font-bold">
								Direct / Unmatched Entry
							</span>
						)}
					</div>

					<Flex.Row
						justify="between"
						className="text-[9.5px] text-(--f3) pt-1.5 border-t border-(--line)/40"
					>
						<span>S3 Trie Candidates:</span>
						<span className="text-(--f1) font-bold">
							{candidates !== undefined
								? `${candidates} matching path(s)`
								: "—"}
						</span>
					</Flex.Row>
				</Flex.Column>
			</Flex.Column>

			{/* 2. Liquidity & Sizing Constraints */}
			<Flex.Column className="border-r border-(--line) min-h-0 bg-(--surface)">
				<Flex.Row
					align="center"
					justify="between"
					className="h-7 px-3 bg-(--sunken) border-b border-(--line) text-[10px] text-(--f3) font-bold uppercase tracking-wider"
				>
					<Flex.Row align="center" gap={1}>
						<Scale className="w-3.5 h-3.5 text-(--info)" />
						<span>Liquidity & Sizing</span>
					</Flex.Row>
					<Badge
						size="xxs"
						variant={binding === "exit_capacity" ? "warning" : "brand"}
						label={binding ? `Bound by ${binding}` : "Sized"}
					/>
				</Flex.Row>

				<Flex.Column className="p-3 flex-1 justify-between gap-1.5">
					{exitCapacity !== undefined ? (
						<Flex.Row
							justify="between"
							align="baseline"
							className="text-[11px]"
						>
							<span className="text-[9.5px] text-(--f4)">
								Book Exit Capacity:
							</span>
							<span className="font-bold text-(--up)">
								{exitCapacity.toFixed(2)} units
							</span>
						</Flex.Row>
					) : null}

					<Flex.Row justify="between" align="baseline" className="text-[11px]">
						<span className="text-[9.5px] text-(--f4)">Cash Allocation:</span>
						<span className="font-bold text-(--f1)">
							{cash !== undefined ? `${cash.toFixed(2)} USD` : "—"}
						</span>
					</Flex.Row>

					{exitCapacity !== undefined ? (
						<Flex.Row
							align="center"
							gap={1}
							className="text-[9px] text-(--up) pt-1.5 border-t border-(--line)/40"
						>
							<ShieldCheck className="w-3 h-3 text-(--up) shrink-0" />
							<span>
								Bid depth within 1.5% of touch caps entry size (carry divisor)
							</span>
						</Flex.Row>
					) : null}
				</Flex.Column>
			</Flex.Column>

			{/* 3. Trie Exit Signal Monitor */}
			<Flex.Column className="min-h-0 bg-(--surface)">
				<Flex.Row
					align="center"
					justify="between"
					className="h-7 px-3 bg-(--sunken) border-b border-(--line) text-[10px] text-(--f3) font-bold uppercase tracking-wider"
				>
					<Flex.Row align="center" gap={1}>
						<Radio
							className={cn(
								"w-3.5 h-3.5",
								isHolding ? "text-(--acc) animate-pulse" : "text-(--f4)",
							)}
						/>
						<span>Trie Exit Monitor</span>
					</Flex.Row>
					<Badge
						size="xxs"
						variant={isClosed ? "success" : isExiting ? "warning" : "brand"}
						className={isExiting ? "animate-pulse" : undefined}
						label={
							isClosed
								? "Position Closed"
								: isExiting
									? "Exit Triggered"
									: "Scanning Trie"
						}
					/>
				</Flex.Row>

				<Flex.Column className="p-3 flex-1 justify-between gap-1.5">
					<div className="py-1">
						{isClosed ? (
							<Flex.Row align="center" gap={2}>
								<Badge
									size="xs"
									variant={trialVariant(closedVerdict(life))}
									label={closedVerdict(life)}
								/>
								<span
									className={cn(
										"font-bold text-[12px]",
										tone(outcome?.pnl ?? 0),
									)}
								>
									{ratePercent(outcome?.pnl ?? 0)} book return
								</span>
							</Flex.Row>
						) : (
							<Flex.Column gap={1}>
								<Flex.Row
									align="center"
									gap={1}
									className="text-[10px] text-(--f2)"
								>
									<Zap className="w-3 h-3 text-(--acc) shrink-0" />
									<span>Monitoring live tokens for exit prefix</span>
								</Flex.Row>
								{mark ? (
									<span
										data-l="lifecycle-mark"
										className={cn(
											"text-[10px] font-bold",
											tone(mark.unrealized),
										)}
										title={`marked ${time(mark.at)} at the bid ${mark.bid}`}
									>
										{signedMoney(mark.unrealized)} USD unrealized (
										{ratePercent(mark.unrealized_return)}) at bid{" "}
										{fieldValue(mark.bid)}
									</span>
								) : null}
								<span className="text-[9px] text-(--f4)">
									Exit rule: policy exit match
								</span>
							</Flex.Column>
						)}
					</div>

					<Flex.Row
						justify="between"
						className="text-[9.5px] text-(--f3) pt-1.5 border-t border-(--line)/40"
					>
						<span>Exit Protocol:</span>
						<span className="text-(--f2)">
							{isClosed
								? `Triggers: ${(outcome?.triggers ?? []).join(", ") || "resolved"}`
								: "Cognition Prefix Engine Active"}
						</span>
					</Flex.Row>
				</Flex.Column>
			</Flex.Column>
		</Grid>
	);
};

/*
A closed virtual lot is a finished search: its A B C ticks index the run's
tape, so the operator labels it up / up_friction / down / chop / flat and
commits it. A commit POSTs to the hub's hindsight commit — the same label
boundary the tape scanner uses — and symm train reads nothing but committed
detections.
*/
const SAMPLE_CLASSES = ["up", "up_friction", "down", "chop", "flat"] as const;

const TrainingSampleCard = ({ life }: { life: Lifecycle }) => {
	const [class_, setClass] = useState<string>("up");
	const [state, setState] = useState<"idle" | "saving" | "saved" | "error">(
		"idle",
	);
	const [error, setError] = useState<string | null>(null);

	if (life.status !== "closed" || life.book !== "virtual") return null;

	const marks =
		life.marks_a !== null &&
		life.marks_a !== undefined &&
		life.marks_b !== null &&
		life.marks_b !== undefined &&
		life.marks_c !== null &&
		life.marks_c !== undefined
			? { a: life.marks_a, b: life.marks_b, c: life.marks_c }
			: null;

	return (
		<Flex.Column
			data-l="lifecycle-training-sample"
			className="border-(--line) border-b bg-(--surface) px-3 py-2 font-mono text-[11px]"
			gap={2}
		>
			<Flex.Row align="center" gap={2} wrap="wrap">
				<span className="font-bold uppercase tracking-wider text-[10px] text-(--f4)">
					training sample
				</span>
				<span className="text-[10px] text-(--f3)">
					A {marks === null ? "—" : marks.a} · B{" "}
					{marks === null ? "—" : marks.b} · C {marks === null ? "—" : marks.c}
					{life.policy_epoch ? ` · run ${life.policy_epoch}` : ""}
				</span>
			</Flex.Row>

			<Flex.Row align="center" gap={2} wrap="wrap">
				{SAMPLE_CLASSES.map((option) => (
					<BadgeButton
						key={option}
						data-l={`sample-class-${option}`}
						size="xs"
						variant={class_ === option ? "brand" : "disabled"}
						className={
							class_ === option ? undefined : "hover:bg-(--sunken) text-(--f3)"
						}
						onClick={() => setClass(option)}
						label={option}
					/>
				))}

				<BadgeButton
					data-l="sample-commit"
					disabled={marks === null || state === "saving"}
					size="xs"
					variant="success"
					className={cn(
						"ml-auto",
						marks === null || state === "saving"
							? "cursor-wait opacity-60"
							: "hover:bg-[color-mix(in_srgb,var(--success)_20%,transparent)]",
					)}
					onClick={async () => {
						if (marks === null || !life.policy_epoch) return;

						setState("saving");
						setError(null);

						try {
							const response = await fetch(`${hubBaseUrl()}/hindsight/commit`, {
								method: "POST",
								headers: { "Content-Type": "application/json" },
								body: JSON.stringify({
									run: life.policy_epoch,
									symbol: life.symbol,
									class: class_,
									a: String(marks.a),
									b: String(marks.b),
									c: String(marks.c),
									d: String(marks.c),
								}),
							});

							if (!response.ok) {
								throw new Error(await response.text());
							}

							setState("saved");
						} catch (cause) {
							setState("error");
							setError(cause instanceof Error ? cause.message : String(cause));
						}
					}}
					label={
						state === "saving"
							? "committing"
							: state === "saved"
								? "committed"
								: "commit"
					}
				/>
			</Flex.Row>

			{marks === null ? (
				<span className="text-[10px] text-(--f4)">
					commit needs entry_tick and exit_tick on the desk lot (B and C); A
					uses mark_a_tick when set, otherwise entry
				</span>
			) : null}
			{marks !== null && !life.policy_epoch ? (
				<span className="text-[10px] text-(--f4)">
					commit needs policy_epoch on the lot (the tick run / workspace epoch
					at entry)
				</span>
			) : null}

			{error !== null ? (
				<span className="text-[10px] text-(--down)">{error}</span>
			) : null}
		</Flex.Column>
	);
};

const OutcomeCard = ({ life }: { life: Lifecycle }) => {
	const outcome = life.outcome;
	if (!outcome || life.status !== "closed") return null;

	const verdict = closedVerdict(life);
	const pnl = outcome.pnl;
	const name = life.book === "virtual" ? "virtual" : "paper";

	const rows: Array<[string, string, number?]> = [
		[`${name} return`, ratePercent(outcome.pnl), outcome.pnl],
		[`${name} return %`, `${outcome.pnl_pct.toFixed(2)}%`, outcome.pnl_pct],
		["exchange P&L", NOT_AVAILABLE],
		["exchange fees", NOT_AVAILABLE],
		[
			"hold / policy hold",
			`${seconds(outcome.hold_ns)} / ${outcome.expected_hold_ns === null ? "—" : seconds(outcome.expected_hold_ns)}`,
		],
		["triggers", (outcome.triggers ?? []).join(", ") || "—"],
	];

	return (
		<Flex.Column
			data-l="lifecycle-outcome"
			className="border-(--line) border-b bg-(--sunken) px-3 py-2.5 font-mono text-[11px]"
		>
			<Flex.Row
				align="center"
				justify="between"
				className="pb-2 mb-2 border-b border-(--line)/40"
			>
				<Flex.Row align="center" gap={2}>
					<Badge size="xs" variant={trialVariant(verdict)} label={verdict} />
					<span className={cn("text-[13px] font-bold", tone(pnl))}>
						{ratePercent(pnl)} book return
					</span>
				</Flex.Row>
				<div className="text-[10px] text-(--f4)">
					Triggers:{" "}
					<span className="text-(--f2)">
						{(outcome.triggers ?? []).map(formatExitTrigger).join(", ") || "—"}
					</span>
				</div>
			</Flex.Row>

			<Grid cols={2} gapX={6} gapY={1} responsive={false}>
				{rows.map(([label, value, sign]) => (
					<Flex.Row key={label} justify="between" gap={2}>
						<span className="text-(--f4)">{label}</span>
						<span className={sign === undefined ? "text-(--f2)" : tone(sign)}>
							{value}
						</span>
					</Flex.Row>
				))}
			</Grid>
		</Flex.Column>
	);
};

/*
LifecycleView is the settled round-trip archive: only closed desk lots appear
in the list. Open lots live on Open positions. Each row replays match, sizing,
fills, exit, and outcome from the hub tape; paper exchange figures read
"not available", never zero.
*/
export const LifecycleView = () => {
	const { report } = useLifecycles();
	const book = useSelector(bookAtom, (state) => state);
	const view = selectBook(report, book);
	const [selected, setSelected] = useState<string | null>(null);
	const lifecycles = view.lifecycles
		.map(shownLife)
		.filter((life) => life.status === "closed")
		.sort((left, right) => {
			const leftAt = left.closed_at ? Date.parse(left.closed_at) : 0;
			const rightAt = right.closed_at ? Date.parse(right.closed_at) : 0;
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
							No closed positions yet. History loads from S3 when the hub is
							connected.
						</Typography.Mono>
					) : (
						lifecycles.map((life) => {
							const pnl = life.outcome
								? life.outcome.pnl
								: life.mark?.unrealized_return;
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
										<span>
											{time(life.opened_at)} · {(life.events ?? []).length}{" "}
											events
										</span>
										{pnl !== undefined ? (
											<span className={cn("font-bold text-[9px]", tone(pnl))}>
												{ratePercent(pnl)}
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
								title={`${active.symbol} · ${trialWord(active) ?? active.status}`}
								meta={`opened ${time(active.opened_at)}${active.closed_at ? ` · ended ${time(active.closed_at)}` : ""}`}
							/>
							<LifecycleStepper life={active} />
							<StrategyIntelligenceCard life={active} />
							<OutcomeCard life={active} />
							<TrainingSampleCard life={active} />
							{(active.events ?? []).map((event, index) => (
								// biome-ignore lint/suspicious/noArrayIndexKey: events are append-only
								<EventRow key={index} event={event} />
							))}
						</>
					) : (
						<Typography.Mono size="s" tone="f3" className="p-3">
							Select a position to see its life cycle.
						</Typography.Mono>
					)}
				</Flex.Column>
			</Flex>
		</Flex.Column>
	);
};
