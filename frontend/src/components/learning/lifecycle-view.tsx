import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { decisionsAtom } from "#/collections/app";
import { Badge, type BadgeVariant } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
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
KIND names each lifecycle event kind and its tone: learned decisions in the
accent, the risk layer in warning, orders and fills neutral, the outcome by
its sign.
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
			.map(([key, inner]) => `${key}:${fieldValue(inner)}`)
			.join(" ");
	}
	return "—";
};

const tone = (value: number) =>
	value > 0 ? "text-(--up)" : value < 0 ? "text-(--down)" : "text-(--f3)";

import {
	Activity,
	CheckCircle2,
	ChevronDown,
	ChevronRight,
	Clock,
	Compass,
	Flag,
	Layers,
	Scale,
	ShieldAlert,
} from "lucide-react";

const EventRow = ({ event }: { event: LifeEvent }) => {
	const [expanded, setExpanded] = useState(false);
	const kind = KIND[event.kind] ?? {
		label: event.kind,
		variant: "disabled" as const,
	};
	const fields = Object.entries(event.fields ?? {});

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
		<div
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
						className="shrink-0 text-[9px] text-(--f4) hover:text-(--f2) flex items-center gap-0.5 border border-(--line) px-1 rounded bg-(--sunken)"
					>
						<span>{fields.length} params</span>
						{expanded ? (
							<ChevronDown className="w-2.5 h-2.5" />
						) : (
							<ChevronRight className="w-2.5 h-2.5" />
						)}
					</button>
				)}
			</Flex.Row>
			{fields.length > 0 ? (
				<div
					className={cn(
						"mt-1.5 ml-6 grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-x-4 gap-y-1 rounded bg-(--sunken)/80 border border-(--line)/40 p-2 text-[10px]",
						!expanded && "hidden",
					)}
				>
					{fields.map(([key, value]) => (
						<div key={key} className="flex min-w-0 justify-between gap-2">
							<span className="text-(--f4)">{key}</span>
							<span className="truncate text-(--f2)" title={fieldValue(value)}>
								{fieldValue(value)}
							</span>
						</div>
					))}
				</div>
			) : null}
		</div>
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
		<div className="flex items-center gap-1.5 px-3 py-2 bg-(--surface) border-b border-(--line) overflow-x-auto text-[10px] font-mono">
			{stages.map((st, idx) => (
				<div key={st.label} className="flex items-center gap-1.5 shrink-0">
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
				</div>
			))}
		</div>
	);
};

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
	const expectedHoldS = sizing?.fields?.expected_hold_s as number | undefined;

	const elapsedS = outcome
		? outcome.hold_ns / 1e9
		: Math.max(0, (Date.now() - new Date(life.opened_at).getTime()) / 1000);
	const targetS = outcome
		? outcome.expected_hold_ns / 1e9
		: expectedHoldS ?? 60;
	const holdPct = Math.min(
		100,
		Math.max(5, (elapsedS / Math.max(1, targetS)) * 100),
	);

	return (
		<div className="border-(--line) border-b bg-(--surface) p-3 font-mono text-[11px]">
			<div className="grid grid-cols-1 md:grid-cols-3 gap-3">
				{/* Strategy Pattern Recognition */}
				<div className="rounded border border-(--line) bg-(--sunken) p-2.5 flex flex-col gap-1.5">
					<div className="flex items-center justify-between text-[10px] text-(--f4) uppercase font-bold tracking-wider">
						<span className="flex items-center gap-1.5">
							<Compass className="w-3.5 h-3.5 text-(--acc)" />
							Strategy Pattern
						</span>
						{confidence !== undefined && (
							<span className="text-(--up) font-bold">{confidence} matched</span>
						)}
					</div>
					<div className="text-[12px] font-bold text-(--acc) truncate">
						{path || "Direct / Unmatched Entry"}
					</div>
					<div className="text-[9.5px] text-(--f3) flex justify-between mt-auto pt-1 border-t border-(--line)/40">
						<span>Trie Candidates:</span>
						<span className="text-(--f1)">
							{candidates !== undefined ? candidates : "—"}
						</span>
					</div>
				</div>

				{/* Liquidity & Capacity Health */}
				<div className="rounded border border-(--line) bg-(--sunken) p-2.5 flex flex-col gap-1.5">
					<div className="flex items-center justify-between text-[10px] text-(--f4) uppercase font-bold tracking-wider">
						<span className="flex items-center gap-1.5">
							<Scale className="w-3.5 h-3.5 text-(--info)" />
							Liquidity & Sizing
						</span>
						<span
							className={cn(
								"px-1 py-0.2 rounded text-[8px] uppercase font-bold",
								binding === "exit_capacity"
									? "bg-(--warn)/15 text-(--warn) border border-(--warn)/30"
									: "bg-(--acc)/15 text-(--acc) border border-(--acc)/30",
							)}
						>
							{binding ? `Bound by ${binding}` : "Sized"}
						</span>
					</div>
					<div className="flex items-baseline justify-between">
						<span className="text-[9.5px] text-(--f4)">Book Exit Capacity:</span>
						<span className="text-[12px] font-bold text-(--up)">
							{exitCapacity !== undefined ? exitCapacity.toFixed(2) : "—"}
						</span>
					</div>
					<div className="text-[9.5px] text-(--f3) flex justify-between mt-auto pt-1 border-t border-(--line)/40">
						<span>Cash Allocation:</span>
						<span className="text-(--f1)">
							{cash !== undefined ? `${cash.toFixed(2)} USD` : "—"}
						</span>
					</div>
				</div>

				{/* Hold Horizon Progress */}
				<div className="rounded border border-(--line) bg-(--sunken) p-2.5 flex flex-col gap-1.5">
					<div className="flex items-center justify-between text-[10px] text-(--f4) uppercase font-bold tracking-wider">
						<span className="flex items-center gap-1.5">
							<Clock className="w-3.5 h-3.5 text-(--acc)" />
							Hold Horizon
						</span>
						<span className="text-(--f2)">
							{elapsedS.toFixed(0)}s / {targetS.toFixed(0)}s
						</span>
					</div>
					<div className="w-full bg-(--surface) border border-(--line) rounded-full h-2 overflow-hidden mt-1">
						<div
							className={cn(
								"h-full transition-all duration-300",
								holdPct >= 100 ? "bg-(--warn)" : "bg-(--acc)",
							)}
							style={{ width: `${holdPct}%` }}
						/>
					</div>
					<div className="text-[9.5px] text-(--f4) flex justify-between mt-auto pt-1 border-t border-(--line)/40">
						<span>Progress:</span>
						<span className="text-(--f2)">
							{holdPct >= 100
								? "Horizon Reached"
								: `${holdPct.toFixed(0)}% elapsed`}
						</span>
					</div>
				</div>
			</div>
		</div>
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
		[
			"hold vs expected",
			`${seconds(outcome.hold_ns)} / ${seconds(outcome.expected_hold_ns)}`,
		],
		["triggers", (outcome.triggers ?? []).join(", ") || "—"],
	];

	if (outcome.shadow_short > 0 || outcome.shadow_unpriced > 0) {
		rows.push([
			"shadow short / unpriced",
			`${outcome.shadow_short} / ${outcome.shadow_unpriced}`,
		]);
	}

	return (
		<div
			data-l="lifecycle-outcome"
			className="border-(--line) border-b bg-(--sunken) px-3 py-2.5 font-mono text-[11px]"
		>
			<div className="flex items-center justify-between pb-2 mb-2 border-b border-(--line)/40">
				<div className="flex items-center gap-2">
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
				</div>
				<div className="text-[10px] text-(--f4)">
					Triggers:{" "}
					<span className="text-(--f2)">
						{(outcome.triggers ?? []).join(", ") || "—"}
					</span>
				</div>
			</div>

			<div className="grid grid-cols-2 gap-x-6 gap-y-1">
				{rows.map(([label, value, sign]) => (
					<div key={label} className="flex justify-between gap-2">
						<span className="text-(--f4)">{label}</span>
						<span className={sign === undefined ? "text-(--f2)" : tone(sign)}>
							{value}
						</span>
					</div>
				))}
			</div>
		</div>
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
						<div
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
						</div>
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
and the outcome. The timeline is the Desk's append-only record, read from
/positions/lifecycle.
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
			<div
				data-l="lifecycle-performance"
				className="flex shrink-0 flex-wrap items-center gap-4 border-(--line) border-b bg-(--surface) px-3 py-1.5 font-mono text-[10px] text-(--f3)"
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
			</div>

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
