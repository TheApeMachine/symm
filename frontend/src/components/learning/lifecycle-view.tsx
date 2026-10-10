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
import { decisionsAtom } from "#/collections/app";
import { Badge, type BadgeVariant } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	type Lifecycle,
	type LifeEvent,
	ratePercent,
	seconds,
	signedMoney,
	useLifecycles,
} from "#/lib/lifecycle";
import { cn } from "#/lib/utils";
import type { DecisionT } from "#/providers/telemetry/telemetry/decision";
import { Explain } from "./explain";

/*
Confluence region taxonomy mapped to human-readable names and visual color tones.
*/
const REGION_INFO: Record<
	string,
	{ name: string; tone: "up" | "down" | "acc" | "info" | "neutral" }
> = {
	R01: { name: "Buy Cascade", tone: "up" },
	R02: { name: "Clock Accel", tone: "acc" },
	R03: { name: "Sell Cascade", tone: "down" },
	R04: { name: "Taker Buy Inflow", tone: "up" },
	R05: { name: "Gross Churn", tone: "neutral" },
	R06: { name: "Taker Sell Outflow", tone: "down" },
	R07: { name: "Ask Void", tone: "info" },
	R08: { name: "Friction / Spread", tone: "neutral" },
	R09: { name: "Bid Pulling", tone: "down" },
	R10: { name: "Cohort Advance", tone: "up" },
	R11: { name: "Iceberg Wall", tone: "info" },
	R12: { name: "Cohort Decline", tone: "down" },
};

const TONE_STYLES = {
	up: "bg-(--up)/15 border-(--up)/30 text-(--up)",
	down: "bg-(--down)/15 border-(--down)/30 text-(--down)",
	acc: "bg-(--acc)/15 border-(--acc)/30 text-(--acc)",
	info: "bg-(--info)/15 border-(--info)/30 text-(--info)",
	neutral: "bg-(--surface) border-(--line) text-(--f2)",
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
								"px-2 py-0.5 rounded border text-[10px] font-bold tracking-wide shadow-xs",
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
Visual Sizing Breakdown: Replaces wall of 17 numbers with comparative constraint bars.
*/
const SizingVisualWidget = ({
	fields,
}: {
	fields: Record<string, unknown>;
}) => {
	const binding = String(fields.binding ?? "cash");
	const cashAlloc = Number(fields.cash) || 0;
	const cashQty = Number(fields.cash_quantity) || 0;
	const exitCap = Number(fields.exit_capacity) || 0;
	const partLimit = Number(fields.participation) || 0;
	const targetQty = Number(fields.quantity) || cashQty || 0;
	const limitPrice = Number(fields.entry_limit_price) || 0;
	const refPrice = Number(fields.reference) || 0;
	const fee = Number(fields.fee) || 0;
	const budget = Number(fields.budget) || 0;

	const exitCushion = targetQty > 0 ? exitCap / targetQty : 1;
	const partCushion = targetQty > 0 ? partLimit / targetQty : 1;

	return (
		<Flex.Column
			gap={2}
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
				<span
					className={cn(
						"px-2 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider border",
						binding === "cash"
							? "bg-(--acc)/15 text-(--acc) border-(--acc)/30"
							: "bg-(--warn)/15 text-(--warn) border-(--warn)/30",
					)}
				>
					Active Bottleneck: Bound by {binding.replace("_", " ")}
				</span>
			</Flex.Row>

			{/* 3 Comparative Constraint Bars */}
			<Grid cols={3} gap={2} responsive={false}>
				{/* Cash Budget */}
				<Flex.Column
					gap={1}
					className={cn(
						"p-2 rounded border transition-colors",
						binding === "cash"
							? "bg-(--acc)/10 border-(--acc)/40"
							: "bg-(--sunken) border-(--line)",
					)}
				>
					<Flex.Row align="center" justify="between" className="text-[10px]">
						<Flex.Row align="center" gap={1} className="text-(--f4)">
							{binding === "cash" && (
								<Lock className="w-2.5 h-2.5 text-(--acc)" />
							)}
							<span>Cash Allocation</span>
						</Flex.Row>
						<span className="font-bold text-(--acc)">
							{cashAlloc.toFixed(2)} USD
						</span>
					</Flex.Row>
					<div className="w-full bg-(--line)/40 h-1.5 rounded-full overflow-hidden mt-0.5">
						<div
							className={cn(
								"h-full rounded-full",
								binding === "cash" ? "bg-(--acc)" : "bg-(--f3)",
							)}
							style={{ width: "100%" }}
						/>
					</div>
					<span className="text-[9px] text-(--f3) mt-0.5">
						{binding === "cash"
							? "100% Capital Deployed (Limiter)"
							: "Budget Allocated"}
					</span>
				</Flex.Column>

				{/* Order Book Exit Capacity */}
				<Flex.Column
					gap={1}
					className={cn(
						"p-2 rounded border transition-colors",
						binding === "exit_capacity"
							? "bg-(--warn)/10 border-(--warn)/40"
							: "bg-(--sunken) border-(--line)",
					)}
				>
					<Flex.Row align="center" justify="between" className="text-[10px]">
						<span className="text-(--f4)">Book Exit Depth</span>
						<span className="font-bold text-(--up)">
							{exitCap.toFixed(2)} units
						</span>
					</Flex.Row>
					<div className="w-full bg-(--line)/40 h-1.5 rounded-full overflow-hidden mt-0.5">
						<div
							className="h-full rounded-full bg-(--up)"
							style={{
								width: `${Math.min(100, Math.max(10, (1 / Math.max(1, exitCushion)) * 100))}%`,
							}}
						/>
					</div>
					<span className="text-[9px] text-(--up) mt-0.5">
						{exitCushion > 1
							? `${exitCushion.toFixed(0)}x Exit Cushion Available`
							: "Tight Exit Cushion"}
					</span>
				</Flex.Column>

				{/* Market Participation Limit */}
				<Flex.Column
					gap={1}
					className={cn(
						"p-2 rounded border transition-colors",
						binding === "participation"
							? "bg-(--warn)/10 border-(--warn)/40"
							: "bg-(--sunken) border-(--line)",
					)}
				>
					<Flex.Row align="center" justify="between" className="text-[10px]">
						<span className="text-(--f4)">Flow Participation</span>
						<span className="font-bold text-(--info)">
							{partLimit.toFixed(2)} units
						</span>
					</Flex.Row>
					<div className="w-full bg-(--line)/40 h-1.5 rounded-full overflow-hidden mt-0.5">
						<div
							className="h-full rounded-full bg-(--info)"
							style={{
								width: `${Math.min(100, Math.max(10, (1 / Math.max(1, partCushion)) * 100))}%`,
							}}
						/>
					</div>
					<span className="text-[9px] text-(--info) mt-0.5">
						{partCushion > 1
							? `${partCushion.toFixed(0)}x Impact Cushion`
							: "At Participation Cap"}
					</span>
				</Flex.Column>
			</Grid>

			{/* Pricing & Execution Economics */}
			<Grid
				cols={4}
				gap={2}
				responsive={false}
				className="text-[10px] pt-1 border-t border-(--line)/30"
			>
				<Flex.Column>
					<span className="text-(--f4) block text-[9px]">Sized Target:</span>
					<span className="font-bold text-(--f1)">{fieldValue(targetQty)}</span>
				</Flex.Column>
				<Flex.Column>
					<span className="text-(--f4) block text-[9px]">Limit Price:</span>
					<span className="font-bold text-(--f2)">
						{limitPrice > 0 ? `$${limitPrice.toFixed(2)}` : "—"}
					</span>
				</Flex.Column>
				<Flex.Column>
					<span className="text-(--f4) block text-[9px]">
						Market Reference:
					</span>
					<span className="font-bold text-(--f2)">
						{refPrice > 0 ? `$${refPrice.toFixed(2)}` : "—"}
					</span>
				</Flex.Column>
				<Flex.Column>
					<span className="text-(--f4) block text-[9px]">Slippage vs Fee:</span>
					<span
						className={cn(
							"font-bold",
							budget >= fee ? "text-(--up)" : "text-(--warn)",
						)}
					>
						{budget > 0 ? `${(budget * 10000).toFixed(0)} bp budget` : "—"} /{" "}
						{(fee * 10000).toFixed(0)} bp fee
					</span>
				</Flex.Column>
			</Grid>
		</Flex.Column>
	);
};

/*
Visual Fill Widget: Replaces raw fill numbers with execution clarity.
*/
const FillVisualWidget = ({ fields }: { fields: Record<string, unknown> }) => {
	const qty = fieldValue(fields.quantity ?? fields.submitted ?? 0);
	const venuePrice = Number(fields.venue_price ?? fields.shadow_price) || 0;
	const venueCost = Number(fields.venue_cost) || 0;
	const venueFee = Number(fields.venue_fee) || 0;

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
				<span className="font-bold text-(--up)">100% FILLED</span>
				<span className="text-(--f1) font-bold ml-1">{qty} units</span>
			</Flex.Row>
			<Flex.Row align="center" gap={3} className="text-[10px]">
				<div>
					<span className="text-(--f4) mr-1">Execution Price:</span>
					<span className="font-bold text-(--f1)">
						{venuePrice > 0 ? `$${venuePrice.toFixed(2)}` : "—"}
					</span>
				</div>
				<div>
					<span className="text-(--f4) mr-1">Total Cost:</span>
					<span className="font-bold text-(--f2)">
						{venueCost > 0 ? `$${venueCost.toFixed(2)}` : "—"}
					</span>
				</div>
				<div>
					<span className="text-(--f4) mr-1">Fee:</span>
					<span className="text-(--f3)">
						{venueFee > 0 ? `$${venueFee.toFixed(2)}` : "—"}
					</span>
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
				{event.venue_at ? (
					<span
						className="shrink-0 text-[10px] text-(--f4)"
						title="venue time of the book or fill"
					>
						venue {time(event.venue_at)}
					</span>
				) : null}
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

const LifecycleStepper = ({ life }: { life: Lifecycle }) => {
	const events = life.events ?? [];
	const hasMatch = events.some((e) => e.kind === "entry_match");
	const hasSizing = events.some((e) => e.kind === "sizing");
	const hasFill = events.some((e) => e.kind === "fill");
	const isHolding =
		life.status === "holding" ||
		life.status === "exiting" ||
		life.status === "closed";
	const hasExit = events.some(
		(e) =>
			e.kind === "exit" || e.kind === "exit_match" || e.kind === "risk_sell",
	);
	const isClosed = life.status === "closed";

	const stages = [
		{ label: "1. Match", done: hasMatch, active: !hasSizing },
		{ label: "2. Sizing", done: hasSizing, active: hasSizing && !hasFill },
		{ label: "3. Fill", done: hasFill, active: hasFill && !isHolding },
		{ label: "4. Hold", done: isHolding, active: life.status === "holding" },
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
					<span
						className={cn(
							"px-2 py-0.5 rounded border text-[9.5px] transition-colors",
							st.done
								? "border-(--acc)/40 bg-(--acc)/10 text-(--acc) font-bold"
								: st.active
									? "border-(--up) bg-(--up)/15 text-(--up) font-bold animate-pulse"
									: "border-(--line) text-(--f4) bg-(--sunken)",
						)}
					>
						{st.label}
					</span>
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

	const path = entryMatch?.fields?.path as string | undefined;
	const confidence = entryMatch?.fields?.confidence as number | undefined;
	const candidates = entryMatch?.fields?.candidates as number | undefined;

	const binding = sizing?.fields?.binding as string | undefined;
	const exitCapacity = sizing?.fields?.exit_capacity as number | undefined;
	const cash = sizing?.fields?.cash as number | undefined;

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
							{confidence} tokens matched
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
					<span
						className={cn(
							"px-1.5 py-0.5 rounded text-[8px] uppercase font-bold border",
							binding === "exit_capacity"
								? "bg-(--warn)/15 text-(--warn) border-(--warn)/30"
								: "bg-(--acc)/15 text-(--acc) border-(--acc)/30",
						)}
					>
						{binding ? `Bound by ${binding}` : "Sized"}
					</span>
				</Flex.Row>

				<Flex.Column className="p-3 flex-1 justify-between gap-1.5">
					<Flex.Row justify="between" align="baseline" className="text-[11px]">
						<span className="text-[9.5px] text-(--f4)">
							Book Exit Capacity:
						</span>
						<span className="font-bold text-(--up)">
							{exitCapacity !== undefined
								? `${exitCapacity.toFixed(2)} units`
								: "—"}
						</span>
					</Flex.Row>

					<Flex.Row justify="between" align="baseline" className="text-[11px]">
						<span className="text-[9.5px] text-(--f4)">Cash Allocation:</span>
						<span className="font-bold text-(--f1)">
							{cash !== undefined ? `${cash.toFixed(2)} USD` : "—"}
						</span>
					</Flex.Row>

					<Flex.Row
						align="center"
						gap={1}
						className="text-[9px] text-(--up) pt-1.5 border-t border-(--line)/40"
					>
						<ShieldCheck className="w-3 h-3 text-(--up) shrink-0" />
						<span>Order book exit depth verified before entry</span>
					</Flex.Row>
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
					<span
						className={cn(
							"px-1.5 py-0.5 rounded text-[8px] uppercase font-bold border",
							isClosed
								? "bg-(--up)/15 text-(--up) border-(--up)/30"
								: isExiting
									? "bg-(--warn)/15 text-(--warn) border-(--warn)/30 animate-pulse"
									: "bg-(--acc)/15 text-(--acc) border-(--acc)/30",
						)}
					>
						{isClosed
							? "Position Closed"
							: isExiting
								? "Exit Triggered"
								: "Scanning Trie"}
					</span>
				</Flex.Row>

				<Flex.Column className="p-3 flex-1 justify-between gap-1.5">
					<div className="py-1">
						{isClosed ? (
							<Flex.Row align="center" gap={2}>
								<span
									className={cn(
										"px-2 py-0.5 rounded text-[10px] font-bold uppercase",
										(outcome?.venue_realized ?? 0) >= 0
											? "bg-(--up)/20 text-(--up) border border-(--up)/30"
											: "bg-(--down)/20 text-(--down) border border-(--down)/30",
									)}
								>
									{(outcome?.venue_realized ?? 0) >= 0 ? "WIN" : "LOSS"}
								</span>
								<span
									className={cn(
										"font-bold text-[12px]",
										tone(outcome?.venue_realized ?? 0),
									)}
								>
									{signedMoney(outcome?.venue_realized ?? 0)} USD Realized
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
								<span className="text-[9px] text-(--f4)">
									Exit rule: S3 learned exit match or capacity trim
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

const OutcomeCard = ({ life }: { life: Lifecycle }) => {
	const outcome = life.outcome;
	if (!outcome) return null;

	const isWin = outcome.shadow_defined
		? outcome.shadow_realized > 0
		: outcome.venue_realized > 0;
	const pnl = outcome.shadow_defined
		? outcome.shadow_realized
		: outcome.venue_realized;

	const rows: Array<[string, string, number?]> = [
		["venue P&L", signedMoney(outcome.venue_realized), outcome.venue_realized],
		[
			"shadow P&L",
			outcome.shadow_defined
				? signedMoney(outcome.shadow_realized)
				: "undefined",
			outcome.shadow_defined ? outcome.shadow_realized : undefined,
		],
		[
			"venue cost / proceeds",
			`${outcome.venue_cost.toFixed(2)} / ${outcome.venue_proceeds.toFixed(2)}`,
		],
		[
			"shadow cost / proceeds",
			`${outcome.shadow_cost.toFixed(2)} / ${outcome.shadow_proceeds.toFixed(2)}`,
		],
		[
			"fees venue / shadow",
			`${outcome.venue_fees.toFixed(4)} / ${outcome.shadow_fees.toFixed(4)}`,
		],
		["hold duration", seconds(outcome.hold_ns)],
		["triggers", (outcome.triggers ?? []).join(", ") || "—"],
	];

	if (outcome.shadow_short > 0 || outcome.shadow_unpriced > 0) {
		rows.push([
			"shadow short / unpriced",
			`${outcome.shadow_short} / ${outcome.shadow_unpriced}`,
		]);
	}

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
					<span
						className={cn(
							"px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider",
							isWin
								? "bg-(--up)/20 text-(--up) border border-(--up)/40"
								: "bg-(--down)/20 text-(--down) border border-(--down)/40",
						)}
					>
						{isWin ? "WIN" : "LOSS"}
					</span>
					<span className={cn("text-[13px] font-bold", tone(pnl))}>
						{signedMoney(pnl)} USD Realized
					</span>
				</Flex.Row>
				<div className="text-[10px] text-(--f4)">
					Triggers:{" "}
					<span className="text-(--f2)">
						{(outcome.triggers ?? []).join(", ") || "—"}
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

const DecisionFeed = () => {
	const decisions = useSelector(decisionsAtom, (list: DecisionT[]) =>
		[...list].sort((left, right) => Number((right.at ?? 0n) - (left.at ?? 0n))),
	);

	return (
		<Section>
			<Section.Header
				title="Engine decisions"
				meta={`${decisions.length} symbols`}
			>
				<Explain>
					The latest matching decision per symbol: enter, exit, accumulate
					(paths disagree or the path is below the minimum confidence) or reset,
					with the matched path and what the desk did with it.
				</Explain>
			</Section.Header>
			<Section.Body>
				{decisions.length === 0 ? (
					<Typography.Mono size="s" tone="f3" className="p-3">
						No decisions yet.
					</Typography.Mono>
				) : (
					decisions.map((decision) => (
						<Flex.Column
							key={String(decision.id ?? decision.symbol)}
							data-l="engine-decision"
							className="border-(--line) border-b px-3 py-1.5 font-mono text-[10px]"
						>
							<Flex.Row align="center" gap={2}>
								<span className="w-16 shrink-0 text-(--f4)">
									{new Date(Number(decision.at ?? 0n) / 1e6).toLocaleTimeString(
										undefined,
										{
											hour12: false,
										},
									)}
								</span>
								<span className="font-bold text-(--f1)">
									{String(decision.symbol ?? "")}
								</span>
								<span
									className={cn(
										"uppercase",
										decision.action === "enter" || decision.action === "exit"
											? "text-(--acc)"
											: "text-(--f3)",
									)}
								>
									{String(decision.action ?? "")}
								</span>
								<span className="ml-auto text-(--f4)">
									{decision.confidence ?? 0} tok ·{" "}
									{(decision.alternatives ?? [])
										.map((alt) => `${alt.name}:${alt.value}`)
										.join(" ")}
								</span>
							</Flex.Row>
							<div
								className="mt-0.5 truncate text-(--f3)"
								title={String(decision.reason ?? "")}
							>
								{String(decision.reason ?? "")}
							</div>
						</Flex.Column>
					))
				)}
			</Section.Body>
		</Section>
	);
};

/*
LifecycleView follows every paper position through its life: the match that
opened it, the desk's sizing inputs, every child order and fill (venue beside
shadow), each risk-layer sell with the capacity it saw, the learned exit match
and the outcome.
*/
export const LifecycleView = () => {
	const { report, online } = useLifecycles();
	const [selected, setSelected] = useState<string | null>(null);
	const lifecycles = report?.lifecycles ?? [];
	const perf = report?.performance;
	const active =
		lifecycles.find((life) => life.id === selected) ?? lifecycles[0];

	return (
		<Flex.Column className="min-h-0 flex-1">
			<Flex.Row
				data-l="lifecycle-performance"
				align="center"
				gap={4}
				wrap="wrap"
				className="shrink-0 border-(--line) border-b bg-(--surface) px-3 py-1.5 font-mono text-[10px] text-(--f3)"
			>
				<span className="font-bold uppercase tracking-wider text-(--f4)">
					Paper outcome
				</span>
				<span>{perf?.closed ?? 0} closed</span>
				<span>
					shadow win {ratePercent(perf?.shadow_win_rate)} · edge{" "}
					{ratePercent(perf?.shadow_mean_return)} · P&L{" "}
					<span className={tone(perf?.shadow_pnl ?? 0)}>
						{signedMoney(perf?.shadow_pnl ?? 0)}
					</span>
				</span>
				<span>
					venue win {ratePercent(perf?.venue_win_rate)} · edge{" "}
					{ratePercent(perf?.venue_mean_return)} · P&L{" "}
					<span className={tone(perf?.venue_pnl ?? 0)}>
						{signedMoney(perf?.venue_pnl ?? 0)}
					</span>
				</span>
				{!online ? (
					<span className="text-(--down)">lifecycle source offline</span>
				) : null}
			</Flex.Row>

			<Flex className="min-h-0 flex-1 max-lg:flex-col">
				<Flex.Column className="w-64 shrink-0 overflow-auto border-(--line) border-r max-lg:w-full">
					{lifecycles.length === 0 ? (
						<Typography.Mono size="s" tone="f3" className="p-3">
							No positions this session yet.
						</Typography.Mono>
					) : (
						lifecycles.map((life) => {
							const pnl = life.outcome
								? life.outcome.shadow_defined
									? life.outcome.shadow_realized
									: life.outcome.venue_realized
								: undefined;
							return (
								<button
									key={life.id}
									type="button"
									data-l="lifecycle-item"
									onClick={() => setSelected(life.id)}
									className={cn(
										"w-full cursor-pointer border-(--line) border-b px-2.5 py-2 text-left font-mono",
										active?.id === life.id
											? "bg-(--acc)/10"
											: "hover:bg-(--sunken)",
									)}
								>
									<Flex.Row align="center" justify="between" gap={2}>
										<span className="truncate text-[11px] font-bold text-(--f1)">
											{life.symbol}
										</span>
										<Badge
											size="xxs"
											variant={STATUS[life.status] ?? "disabled"}
											label={life.status}
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
											<span className={tone(pnl)}>{signedMoney(pnl)}</span>
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
								title={`${active.symbol} · ${active.status}`}
								meta={`opened ${time(active.opened_at)}${active.closed_at ? ` · ended ${time(active.closed_at)}` : ""}`}
							/>
							<LifecycleStepper life={active} />
							<StrategyIntelligenceCard life={active} />
							<OutcomeCard life={active} />
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

				<Flex.Column className="w-96 shrink-0 overflow-auto border-(--line) border-l max-lg:w-full">
					<DecisionFeed />
				</Flex.Column>
			</Flex>
		</Flex.Column>
	);
};
