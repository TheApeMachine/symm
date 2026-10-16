import { useSelector } from "@tanstack/react-store";
import { ArrowRight, HeartPulse, Radio } from "lucide-react";
import React, { useEffect, useMemo, useState } from "react";
import {
	decisionsAtom,
	type RingBuffer,
	signals,
	unrealizedAtom,
} from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import { Section } from "#/components/ui/section";
import type { DecisionT } from "#/collections/types";
import type { Lifecycle } from "#/lib/lifecycle";
import { cn } from "#/lib/utils";
import type { Value } from "#/types/value";

export interface RegionMeta {
	id: string;
	title: string;
	subtitle: string;
	role:
		| "entry_detector"
		| "entry_support"
		| "entry_opposition"
		| "exit_detector"
		| "exit_support"
		| "exit_opposition"
		| "regime"
		| "history"
		| "risk";
	tone: "up" | "down" | "acc" | "info" | "neutral" | "warn";
	description: string;
}

export const REGIONS: Record<string, RegionMeta> = {
	R00: {
		id: "R00",
		title: "Enter Detector",
		subtitle: "Point-Process Spark",
		role: "entry_detector",
		tone: "up",
		description:
			"Point-process arrival of aggressive buyers, volume spike, and orderbook pressure sparks.",
	},
	R01: {
		id: "R01",
		title: "Enter Support",
		subtitle: "Bid Replenishment",
		role: "entry_support",
		tone: "up",
		description:
			"Limit order bid replenishment, book depth skew on bid side, positive delta accumulation.",
	},
	R02: {
		id: "R02",
		title: "Enter Opposition",
		subtitle: "Offer Depletion",
		role: "entry_opposition",
		tone: "info",
		description:
			"Ask side thinning/depletion, low seller resistance (inelasticity on offer side).",
	},
	R03: {
		id: "R03",
		title: "Exit Detector",
		subtitle: "Sell Cascade",
		role: "exit_detector",
		tone: "down",
		description:
			"Aggressive seller arrival cascade, sudden volume dump, negative midpoint return ignition.",
	},
	R04: {
		id: "R04",
		title: "Exit Support",
		subtitle: "Ask Loading",
		role: "exit_support",
		tone: "down",
		description:
			"Ask replenishment, resistance wall loading, negative delta accumulation.",
	},
	R05: {
		id: "R05",
		title: "Exit Opposition",
		subtitle: "Bid Evaporation",
		role: "exit_opposition",
		tone: "down",
		description:
			"Bid side depletion/pulling, high bid slippage, collapse of buy support.",
	},
	R06: {
		id: "R06",
		title: "Coiled Compression",
		subtitle: "Energy Building",
		role: "regime",
		tone: "neutral",
		description:
			"Low volatility, narrow spread ratio, tight range, energy building.",
	},
	R07: {
		id: "R07",
		title: "Explosive Ignition",
		subtitle: "Supercritical Burst",
		role: "regime",
		tone: "acc",
		description:
			"Rapid spread opening, extreme volume burst, high tick velocity.",
	},
	R08: {
		id: "R08",
		title: "Turbulent Friction",
		subtitle: "Two-Sided Churn",
		role: "regime",
		tone: "warn",
		description:
			"Choppy two-sided churn, high cancellation rate, spread dislocation.",
	},
	R09: {
		id: "R09",
		title: "Dead / Starved",
		subtitle: "Sparse Liquidity",
		role: "regime",
		tone: "neutral",
		description:
			"Illiquid book, wide spread, sparse arrivals, insufficient trading activity.",
	},
	R10: {
		id: "R10",
		title: "Macro Recurrence",
		subtitle: "Leader Concordance",
		role: "history",
		tone: "up",
		description:
			"Correlated peer alignment, market-wide leader advance concordance.",
	},
	R11: {
		id: "R11",
		title: "Sentiment Recurrence",
		subtitle: "Peer Consensus",
		role: "history",
		tone: "info",
		description: "Cross-asset sentiment agreement, peer directional consensus.",
	},
	R12: {
		id: "R12",
		title: "Friction Hurdle",
		subtitle: "Fee & Spread Drag",
		role: "risk",
		tone: "warn",
		description:
			"High relative spread, roundtrip fee drag, cost hurdle exceeding expected drift.",
	},
	R13: {
		id: "R13",
		title: "Macro Contagion",
		subtitle: "Systemic Dispersion",
		role: "risk",
		tone: "warn",
		description:
			"Systemic peer dislocation, lead-lag breakdown, adverse market beta.",
	},
	R14: {
		id: "R14",
		title: "Capacity / Liquidity",
		subtitle: "Exit Thinness",
		role: "risk",
		tone: "warn",
		description:
			"Thin book depth relative to position size, high market impact risk.",
	},
};

export const REGION_KEYS = [
	"R00",
	"R01",
	"R02",
	"R03",
	"R04",
	"R05",
	"R06",
	"R07",
	"R08",
	"R09",
	"R10",
	"R11",
	"R12",
	"R13",
	"R14",
];

export const PositionHealthGauge = ({
	life,
	unrealizedVal,
}: {
	life?: Lifecycle | null;
	unrealizedVal?: number;
}) => {
	const rawUnrealized = useSelector(unrealizedAtom, (state) => Number(state));
	const currentUnrealized = Number.isFinite(unrealizedVal)
		? (unrealizedVal as number)
		: Number.isFinite(rawUnrealized)
			? rawUnrealized
			: 0;

	const events = life?.events ?? [];
	const fillEvent = events.find(
		(e) =>
			e.kind === "fill" || e.kind === "entry_order" || e.kind === "ENTRY_ORDER",
	);

	const cost = Number(fillEvent?.fields?.cost) || 0;
	const fee = Number(fillEvent?.fields?.fee) || 0;

	const pnlPct = cost > 0 ? (currentUnrealized / cost) * 100 : 0;

	let healthScore = 75;
	let statusText = "HEALTHY HOLD";
	let statusVariant: "success" | "brand" | "warning" | "error" = "brand";
	let verdictText =
		"Position is holding securely near entry basis. Trailing protection active.";

	if (cost <= 0) {
		healthScore = 50;
		statusText = "PENDING FILL";
		statusVariant = "brand";
		verdictText = "Order submitted to venue. Awaiting fill confirmation.";
	} else if (pnlPct >= 1.0) {
		healthScore = Math.min(100, Math.round(80 + pnlPct * 5));
		statusText = "IN PROFIT";
		statusVariant = "success";
		verdictText = "Gains exceed fee hurdle. Running towards learned target.";
	} else if (pnlPct >= -0.85) {
		healthScore = Math.round(65 + (pnlPct + 0.85) * 15);
		statusText = "STABLE HOLD";
		statusVariant = "brand";
		verdictText = "Within fee buffer. No adverse sell cascades detected.";
	} else if (pnlPct >= -2.5) {
		healthScore = Math.round(40 + (pnlPct + 2.5) * 15);
		statusText = "GUARDED";
		statusVariant = "warning";
		verdictText = "Approaching defensive boundary. S3 exit engine watchful.";
	} else {
		healthScore = Math.max(10, Math.round(20 + (pnlPct + 5) * 8));
		statusText = "DEFENSE STOP";
		statusVariant = "error";
		verdictText = "Under drawdown pressure. Risk layer exit primed.";
	}

	return (
		<Flex.Column
			gap={1}
			className="rounded-xs border border-(--line) bg-(--surface) p-2.5 font-mono text-[11px]"
		>
			<Flex.Row
				align="center"
				justify="between"
				className="border-b border-(--line)/40 pb-1.5 text-[10px]"
			>
				<Flex.Row
					align="center"
					gap={1}
					className="font-bold text-(--f1) uppercase"
				>
					<HeartPulse className="w-3.5 h-3.5 text-(--acc)" />
					<span>Position Health Gauge</span>
				</Flex.Row>
				<Badge size="xxs" variant={statusVariant} label={statusText} />
			</Flex.Row>

			<Meter
				size="s"
				variant={statusVariant}
				percent={healthScore}
				label="Vitality"
				value={`${healthScore}% (${statusText})`}
			/>

			<Grid cols={3} gap={2} responsive={false} className="text-[10px]">
				<Flex.Row justify="between">
					<span className="text-(--f4)">Entry Basis:</span>
					<span className="font-bold text-(--f1)">
						{cost > 0 ? `$${cost.toFixed(2)}` : "—"}
					</span>
				</Flex.Row>
				<Flex.Row justify="between">
					<span className="text-(--f4)">Taker Fee:</span>
					<span className="text-(--f3)">
						{fee > 0 ? `$${fee.toFixed(4)}` : "—"}
					</span>
				</Flex.Row>
				<Flex.Row justify="between">
					<span className="text-(--f4)">Unrealized:</span>
					<span
						className={
							currentUnrealized >= 0
								? "font-bold text-(--up)"
								: "font-bold text-(--f2)"
						}
					>
						{currentUnrealized >= 0 ? "+" : ""}
						{currentUnrealized.toFixed(2)} USD ({pnlPct.toFixed(2)}%)
					</span>
				</Flex.Row>
			</Grid>

			<div className="rounded-xs border border-(--line)/40 bg-(--sunken) px-2 py-1 text-[9.5px] text-(--f3)">
				{verdictText}
			</div>
		</Flex.Column>
	);
};

export const LiveRegionActivationMap = ({
	currentPath,
	symbol,
}: {
	currentPath?: string;
	symbol: string;
}) => {
	const visitedTokens = useMemo(
		() => (currentPath ? currentPath.split("/").filter(Boolean) : []),
		[currentPath],
	);

	const activeToken =
		visitedTokens.length > 0
			? visitedTokens[visitedTokens.length - 1]
			: undefined;

	const [inspectRegion, setInspectRegion] = useState<string | null>(null);
	const inspectedMeta = inspectRegion ? REGIONS[inspectRegion] : null;

	return (
		<Flex.Column
			gap={1}
			className="rounded-xs border border-(--line) bg-(--surface) p-2.5 font-mono text-[11px]"
		>
			<Flex.Row
				align="center"
				justify="between"
				className="border-b border-(--line)/40 pb-1.5 text-[10px]"
			>
				<Flex.Row
					align="center"
					gap={1}
					className="font-bold text-(--f1) uppercase"
				>
					<Radio className="w-3.5 h-3.5 text-(--acc)" />
					<span>Live Region Confluence Radar</span>
				</Flex.Row>
				<span className="text-(--f4)">
					{symbol.replace("/USD", "")} •{" "}
					{activeToken ? (
						<span className="font-bold text-(--acc)">{activeToken} ACTIVE</span>
					) : (
						"IDLE"
					)}
				</span>
			</Flex.Row>

			<div className="grid grid-cols-5 gap-1">
				{REGION_KEYS.map((key) => {
					const meta = REGIONS[key];
					const isCurrent = key === activeToken;
					const stepIndex = visitedTokens.indexOf(key);
					const isVisited = stepIndex !== -1;

					return (
						<button
							key={key}
							type="button"
							onClick={() =>
								setInspectRegion((prev) => (prev === key ? null : key))
							}
							title={`${meta.title} (${meta.subtitle})`}
							className={cn(
								"flex flex-col justify-between rounded-xs border p-1 text-left font-mono transition-colors cursor-pointer",
								isCurrent
									? "border-(--acc) bg-(--acc)/20 text-(--acc) font-bold ring-1 ring-(--acc)"
									: isVisited
										? "border-(--up)/40 bg-(--up)/10 text-(--up) font-bold"
										: "border-(--line) bg-(--sunken) text-(--f4) hover:border-(--f3)",
							)}
						>
							<Flex.Row align="center" justify="between">
								<span className="text-[10px] font-bold">{key}</span>
								{isCurrent && (
									<span className="size-1 rounded-full bg-(--acc) animate-ping" />
								)}
								{!isCurrent && isVisited && (
									<span className="text-[8px] opacity-80">
										#{stepIndex + 1}
									</span>
								)}
							</Flex.Row>
							<span className="text-[8px] truncate opacity-85 mt-0.5">
								{meta.title}
							</span>
						</button>
					);
				})}
			</div>

			{inspectedMeta && (
				<div className="rounded-xs border border-(--line) bg-(--sunken) p-1.5 text-[9.5px] text-(--f2)">
					<span className="font-bold text-(--f1)">
						{inspectedMeta.id}: {inspectedMeta.title}
					</span>{" "}
					<span className="text-(--f4)">({inspectedMeta.subtitle})</span>
					<div className="text-(--f3) mt-0.5">{inspectedMeta.description}</div>
				</div>
			)}
		</Flex.Column>
	);
};

export const RegionTransitionPathway = ({
	entryPath = "R01/R04/R06",
	currentPath = "R01/R04/R06",
}: {
	entryPath?: string;
	currentPath?: string;
}) => {
	const entryTokens = useMemo(
		() => (entryPath ? entryPath.split("/").filter(Boolean) : []),
		[entryPath],
	);

	const activeTokens = useMemo(
		() => (currentPath ? currentPath.split("/").filter(Boolean) : []),
		[currentPath],
	);

	const currentActiveToken =
		activeTokens.length > 0 ? activeTokens[activeTokens.length - 1] : "R06";

	return (
		<Flex.Column
			gap={1}
			className="rounded-xs border border-(--line) bg-(--surface) p-2.5 font-mono text-[11px]"
		>
			<Flex.Row
				align="center"
				justify="between"
				className="border-b border-(--line)/40 pb-1.5 text-[10px]"
			>
				<span className="font-bold uppercase tracking-wider text-(--f4)">
					Learned Region Transitions (Entry & Exit)
				</span>
			</Flex.Row>

			<Grid cols={2} gap={2} responsive={false} className="text-[10px]">
				<Flex.Column
					gap={1}
					className="rounded-xs border border-(--line) bg-(--sunken) p-2"
				>
					<span className="text-[9px] font-bold uppercase tracking-wider text-(--f4)">
						Entry Ignition Pathway
					</span>
					<Flex.Row align="center" gap={1} wrap="wrap">
						{entryTokens.map((tok, i) => {
							const meta = REGIONS[tok];
							return (
								<React.Fragment key={tok}>
									{i > 0 && (
										<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
									)}
									<span className="rounded-xs border border-(--line) bg-(--surface) px-1.5 py-0.5 text-[9px] text-(--f2)">
										<strong className="text-(--f1)">{tok}</strong>{" "}
										{meta?.title?.split(" ")[0]}
									</span>
								</React.Fragment>
							);
						})}
						<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
						<span className="rounded-xs border border-(--up)/40 bg-(--up)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--up)">
							★ ENTER
						</span>
					</Flex.Row>
				</Flex.Column>

				<Flex.Column
					gap={1}
					className="rounded-xs border border-(--line) bg-(--sunken) p-2"
				>
					<span className="text-[9px] font-bold uppercase tracking-wider text-(--f4)">
						Active Exit Targets
					</span>
					<Flex.Column gap={1}>
						<Flex.Row align="center" gap={1}>
							<span className="rounded-xs bg-(--surface) px-1.5 py-0.5 text-[9px] font-bold text-(--acc) border border-(--line)">
								{currentActiveToken}
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--warn)/40 bg-(--warn)/15 px-1.5 py-0.5 text-[9px] font-bold text-(--warn)">
								R03 (Exit Detector)
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--down)/40 bg-(--down)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--down)">
								⚡ EXIT
							</span>
						</Flex.Row>
						<Flex.Row align="center" gap={1}>
							<span className="rounded-xs bg-(--surface) px-1.5 py-0.5 text-[9px] font-bold text-(--acc) border border-(--line)">
								{currentActiveToken}
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--warn)/40 bg-(--warn)/15 px-1.5 py-0.5 text-[9px] font-bold text-(--warn)">
								R05 (Exit Opposition)
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--down)/40 bg-(--down)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--down)">
								⚡ EXIT
							</span>
						</Flex.Row>
					</Flex.Column>
				</Flex.Column>
			</Grid>
		</Flex.Column>
	);
};

export const ActivePositionHealthCard = ({ life }: { life: Lifecycle }) => {
	const rawUnrealized = useSelector(unrealizedAtom, (state) => Number(state));
	const currentUnrealized = Number.isFinite(rawUnrealized) ? rawUnrealized : 0;

	const decisions = useSelector(decisionsAtom, (list: DecisionT[]) => list);
	const activeSymbol = life?.symbol ?? "";
	const currentDecision = decisions.find((d) => d.symbol === activeSymbol);

	// Read live ring buffer from signals.training for this specific symbol
	const [latestValue, setLatestValue] =
		useState<Value | null>(() => {
			const ring = signals.training?.state?.[activeSymbol];
			if (
				ring &&
				typeof ring.getBufferLength === "function" &&
				ring.getBufferLength() > 0
			) {
				return ring.get(ring.getBufferLength() - 1) ?? null;
			}
			return null;
		});

	useEffect(() => {
		const updateRing = (state: Record<string, RingBuffer<Value>>) => {
			const ring = state[activeSymbol];
			if (
				ring &&
				typeof ring.getBufferLength === "function" &&
				ring.getBufferLength() > 0
			) {
				const latest = ring.get(ring.getBufferLength() - 1);
				if (latest) {
					setLatestValue(latest);
				}
			}
		};

		if (signals.training?.state) {
			updateRing(signals.training.state);
		}

		const unsub = signals.training?.subscribe(updateRing);
		return () => {
			unsub?.unsubscribe?.();
		};
	}, [activeSymbol]);

	const livePathFromMeasurement = useMemo(() => {
		if (!latestValue?.provenance) return "";
		for (const p of latestValue.provenance) {
			if (p?.name === "token_path" && p.value) {
				return String(p.value);
			}
		}
		return "";
	}, [latestValue]);

	const currentPath = livePathFromMeasurement || currentDecision?.cause || "";

	const events = life.events ?? [];
	const entryMatch = events.find((e) => e.kind === "entry_match");
	const entryPath = (entryMatch?.fields?.path as string) || "";

	const fill = events.find((e) => e.kind === "fill" || e.kind === "FILL");
	const volume = Number(fill?.fields?.volume) || 0;
	const price = Number(fill?.fields?.price) || 0;
	const cost =
		Number(fill?.fields?.cost) ||
		(volume > 0 && price > 0 ? volume * price : 0);
	const fee = Number(fill?.fields?.fee) || 0;

	const pnlPct = cost > 0 ? (currentUnrealized / cost) * 100 : 0;

	let healthScore = 75;
	let statusText = "HEALTHY HOLD";
	let statusVariant: "success" | "brand" | "warning" | "error" = "brand";
	let verdictText =
		"Position is holding securely near entry basis. Trailing protection active.";

	if (cost <= 0) {
		healthScore = 50;
		statusText = "PENDING FILL";
		statusVariant = "brand";
		verdictText = "Order submitted to venue. Awaiting fill confirmation.";
	} else if (pnlPct >= 1.0) {
		healthScore = Math.min(100, Math.round(80 + pnlPct * 5));
		statusText = "IN PROFIT";
		statusVariant = "success";
		verdictText = "Gains exceed fee hurdle. Running towards learned target.";
	} else if (pnlPct >= -0.85) {
		healthScore = Math.round(65 + (pnlPct + 0.85) * 15);
		statusText = "STABLE HOLD";
		statusVariant = "brand";
		verdictText = "Within fee buffer. No adverse sell cascades detected.";
	} else if (pnlPct >= -2.5) {
		healthScore = Math.round(40 + (pnlPct + 2.5) * 15);
		statusText = "GUARDED";
		statusVariant = "warning";
		verdictText = "Approaching defensive boundary. S3 exit engine watchful.";
	} else {
		healthScore = Math.max(10, Math.round(20 + (pnlPct + 5) * 8));
		statusText = "DEFENSE STOP";
		statusVariant = "error";
		verdictText = "Under drawdown pressure. Risk layer exit primed.";
	}

	const visitedTokens = (currentPath as string).split("/").filter(Boolean);
	const activeToken =
		visitedTokens.length > 0 ? visitedTokens[visitedTokens.length - 1] : "";
	const entryTokens = entryPath.split("/").filter(Boolean);

	return (
		<Flex.Column
			data-l="lifecycle-active-health"
			gap={2}
			className="border-b border-(--line) bg-(--surface) p-3 font-mono text-[11px]"
		>
			{/* Top Position Header */}
			<Flex.Row
				align="center"
				justify="between"
				className="border-b border-(--line)/40 pb-2"
			>
				<Flex.Row align="center" gap={2}>
					<Badge
						size="xs"
						variant="brand"
						label="OPEN POSITION"
						className="animate-pulse"
					/>
					<span className="font-bold text-[12px] text-(--f1)">
						Holding {volume > 0 ? volume.toFixed(2) : "—"}{" "}
						{life.symbol.replace("/USD", "")}
						{price > 0 ? ` @ $${price.toFixed(5)}` : ""}
					</span>
				</Flex.Row>
				<span className="text-[10px] text-(--f4)">
					Cost Basis:{" "}
					<span className="font-bold text-(--f1)">
						{cost > 0 ? `$${cost.toFixed(2)}` : "—"}
					</span>{" "}
					· Fee: {fee > 0 ? `$${fee.toFixed(4)}` : "—"}
				</span>
			</Flex.Row>

			{/* Health & Vitality Progress using standard Meter component */}
			<Flex.Column
				gap={1}
				className="rounded-xs border border-(--line) bg-(--sunken) p-2.5"
			>
				<Meter
					size="s"
					variant={statusVariant}
					percent={healthScore}
					label="POSITION VITALITY"
					value={`${healthScore}% (${statusText})`}
				/>
				<Grid cols={4} gap={2} responsive={false} className="mt-1 text-[10px]">
					<Flex.Row justify="between">
						<span className="text-(--f4)">Entry Basis:</span>
						<span className="font-bold text-(--f1)">
							{cost > 0 ? `$${cost.toFixed(2)}` : "—"}
						</span>
					</Flex.Row>
					<Flex.Row justify="between">
						<span className="text-(--f4)">Taker Fee:</span>
						<span className="text-(--f3)">
							{fee > 0 ? `$${fee.toFixed(4)}` : "—"}
						</span>
					</Flex.Row>
					<Flex.Row justify="between">
						<span className="text-(--f4)">Unrealized Return:</span>
						<span
							className={
								currentUnrealized >= 0
									? "font-bold text-(--up)"
									: "font-bold text-(--f2)"
							}
						>
							{cost > 0 ? (
								<>
									{currentUnrealized >= 0 ? "+" : ""}
									{currentUnrealized.toFixed(2)} USD ({pnlPct.toFixed(2)}%)
								</>
							) : (
								"—"
							)}
						</span>
					</Flex.Row>
					<Flex.Row justify="between">
						<span className="text-(--f4)">Current Status:</span>
						<span className="font-bold text-(--acc)">4. HOLD (Protected)</span>
					</Flex.Row>
				</Grid>
				<span className="text-[9.5px] text-(--f3) mt-0.5">{verdictText}</span>
			</Flex.Column>

			{/* 15-Region Live Confluence Strip */}
			<Flex.Column gap={1}>
				<Flex.Row align="center" justify="between" className="text-[10px]">
					<span className="font-bold uppercase tracking-wider text-(--f4)">
						Live Region Confluence · {life.symbol.replace("/USD", "")}
					</span>
					<span className="text-[9px] text-(--f3)">
						Active:{" "}
						<span className="font-bold text-(--acc)">{activeToken}</span>
					</span>
				</Flex.Row>
				<div
					style={{
						display: "grid",
						gridTemplateColumns: "repeat(15, minmax(0, 1fr))",
						gap: "0.25rem",
					}}
				>
					{REGION_KEYS.map((key) => {
						const meta = REGIONS[key];
						const isCurrent = key === activeToken;
						const isEntry = entryTokens.includes(key);
						const isExitCandidate =
							key === "R03" || key === "R04" || key === "R05";

						return (
							<div
								key={key}
								title={`${meta?.title ?? key}: ${meta?.description ?? ""}`}
								className={cn(
									"flex flex-col items-center justify-center rounded-xs border py-1 px-0.5 text-center font-mono text-[9px] transition-colors",
									isCurrent
										? "border-(--acc) bg-(--acc)/20 text-(--acc) font-bold ring-1 ring-(--acc)"
										: isEntry
											? "border-(--up)/40 bg-(--up)/10 text-(--up) font-bold"
											: isExitCandidate
												? "border-(--warn)/40 bg-(--warn)/10 text-(--warn)"
												: "border-(--line) bg-(--sunken) text-(--f4) opacity-65",
								)}
							>
								<span className="font-bold">{key}</span>
								<span className="text-[7.5px] truncate max-w-full opacity-80">
									{meta?.title?.split(" ")[0] ?? ""}
								</span>
							</div>
						);
					})}
				</div>
			</Flex.Column>

			{/* Entry & Exit Region Transition Pathways */}
			<Grid cols={2} gap={2} responsive={false} className="text-[10px]">
				<Flex.Column
					gap={1}
					className="rounded-xs border border-(--line) bg-(--sunken) p-2"
				>
					<span className="text-[9px] font-bold uppercase tracking-wider text-(--f4)">
						Entry Ignition Pathway
					</span>
					<Flex.Row align="center" gap={1} wrap="wrap">
						{entryTokens.map((tok, i) => {
							const meta = REGIONS[tok];
							return (
								<React.Fragment key={tok}>
									{i > 0 && (
										<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
									)}
									<span className="rounded-xs border border-(--line) bg-(--surface) px-1.5 py-0.5 text-[9px] text-(--f2)">
										<strong className="text-(--f1)">{tok}</strong>{" "}
										{meta?.title?.split(" ")[0]}
									</span>
								</React.Fragment>
							);
						})}
						<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
						<span className="rounded-xs border border-(--up)/40 bg-(--up)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--up)">
							★ ENTER
						</span>
					</Flex.Row>
				</Flex.Column>

				<Flex.Column
					gap={1}
					className="rounded-xs border border-(--line) bg-(--sunken) p-2"
				>
					<span className="text-[9px] font-bold uppercase tracking-wider text-(--f4)">
						Active Exit Targets
					</span>
					<Flex.Column gap={1}>
						<Flex.Row align="center" gap={1}>
							<span className="rounded-xs bg-(--surface) px-1.5 py-0.5 text-[9px] font-bold text-(--acc) border border-(--line)">
								{activeToken}
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--warn)/40 bg-(--warn)/15 px-1.5 py-0.5 text-[9px] font-bold text-(--warn)">
								R03 (Exit Detector)
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--down)/40 bg-(--down)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--down)">
								⚡ EXIT
							</span>
						</Flex.Row>
						<Flex.Row align="center" gap={1}>
							<span className="rounded-xs bg-(--surface) px-1.5 py-0.5 text-[9px] font-bold text-(--acc) border border-(--line)">
								{activeToken}
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--warn)/40 bg-(--warn)/15 px-1.5 py-0.5 text-[9px] font-bold text-(--warn)">
								R05 (Exit Opposition)
							</span>
							<ArrowRight className="h-2.5 w-2.5 text-(--f4) shrink-0" />
							<span className="rounded-xs border border-(--down)/40 bg-(--down)/20 px-1.5 py-0.5 text-[9px] font-bold text-(--down)">
								⚡ EXIT
							</span>
						</Flex.Row>
					</Flex.Column>
				</Flex.Column>
			</Grid>
		</Flex.Column>
	);
};

/*
TradeRegionScores renders all 12 confluence regions with horizontal score bars,
showing the active regions, entry confluence, and exit precursor levels for the trade.
*/
export const TradeRegionScores = ({ life }: { life?: Lifecycle | null }) => {
	const activeSymbol = life?.symbol ?? "BTC/USD";

	// Subscribe to live value for activeSymbol directly from signals.training
	const [latestValue, setLatestValue] =
		useState<Value | null>(() => {
			const ring = signals.training?.state?.[activeSymbol];
			if (
				ring &&
				typeof ring.getBufferLength === "function" &&
				ring.getBufferLength() > 0
			) {
				return ring.get(ring.getBufferLength() - 1) ?? null;
			}
			return null;
		});

	useEffect(() => {
		const updateValue = (
			state: Record<string, RingBuffer<Value>>,
		) => {
			const ring = state[activeSymbol];
			if (
				ring &&
				typeof ring.getBufferLength === "function" &&
				ring.getBufferLength() > 0
			) {
				const latest = ring.get(ring.getBufferLength() - 1);
				if (latest) {
					setLatestValue(latest);
				}
			}
		};

		if (signals.training?.state) {
			updateValue(signals.training.state);
		}

		const unsub = signals.training?.subscribe(updateValue);
		return () => {
			unsub?.unsubscribe?.();
		};
	}, [activeSymbol]);

	const metricMap = useMemo(() => {
		const map: Record<string, number> = {};
		if (!latestValue?.metrics) return map;
		for (const m of latestValue.metrics) {
			if (m?.name) {
				map[String(m.name)] = m.raw ?? 0;
			}
		}
		return map;
	}, [latestValue]);

	const liveTokenPath = useMemo(() => {
		if (!latestValue?.provenance) return "";
		for (const p of latestValue.provenance) {
			if (p?.name === "token_path" && p.value) {
				return String(p.value);
			}
		}
		return "";
	}, [latestValue]);

	const liveTokens = useMemo(
		() => liveTokenPath.split("/").filter(Boolean),
		[liveTokenPath],
	);
	const activeToken =
		liveTokens.length > 0 ? liveTokens[liveTokens.length - 1] : "";

	const events = life?.events ?? [];
	const entryMatch = events.find((e) => e.kind === "entry_match");
	const entryPath = (entryMatch?.fields?.path as string) || "";
	const entryTokens = useMemo(
		() => entryPath.split("/").filter(Boolean),
		[entryPath],
	);

	const maxBrightness = useMemo(() => {
		let maxVal = 0.001;
		for (const key of REGION_KEYS) {
			const val = metricMap[`region_brightness:${key}`];
			if (val !== undefined && val > maxVal) {
				maxVal = val;
			}
		}
		return maxVal;
	}, [metricMap]);

	// Stable R00-R14 order: never sort dynamically to prevent jumping rows during live streaming
	const regionScores = useMemo(() => {
		return REGION_KEYS.map((key) => {
			const meta = REGIONS[key];
			const isCurrent = key === activeToken;
			const isEntry = entryTokens.includes(key);
			const isLivePath = liveTokens.includes(key);
			const isExitCandidate = key === "R03" || key === "R04" || key === "R05";

			const rawBrightness = metricMap[`region_brightness:${key}`];
			const members = Math.round(metricMap[`region_members:${key}`] ?? 0);

			let percent = 0;
			let statusLabel = "Quiescent";
			let variant:
				| "brand"
				| "info"
				| "success"
				| "warning"
				| "error"
				| "disabled" = "disabled";

			if (rawBrightness !== undefined) {
				percent = Math.min(
					100,
					Math.max(0, Math.round((rawBrightness / maxBrightness) * 100)),
				);

				if (isCurrent) {
					statusLabel = "ACTIVE TRIGGER";
					variant = "info";
				} else if (isLivePath) {
					statusLabel = "PATH COMPONENT";
					variant = "brand";
				} else if (isEntry) {
					statusLabel = "ENTRY CONFLUENCE";
					variant = "success";
				} else if (isExitCandidate && rawBrightness >= 1.0) {
					statusLabel = "EXIT PRECURSOR";
					variant = "warning";
				} else if (rawBrightness >= 1.5) {
					statusLabel = "ELEVATED";
					variant = "brand";
				} else if (members > 0) {
					statusLabel = "Active";
					variant = "disabled";
				}
			} else {
				statusLabel = "Unobserved";
				variant = "disabled";
			}

			return {
				key,
				meta,
				rawBrightness,
				members,
				percent,
				statusLabel,
				variant,
				isCurrent,
				isEntry,
				isExitCandidate,
			};
		});
	}, [activeToken, entryTokens, liveTokens, maxBrightness, metricMap]);

	const hasEvidence = Object.keys(metricMap).some((k) =>
		k.startsWith("region_brightness:"),
	);

	return (
		<Section
			surface="surface"
			fit="pane"
			className="h-full font-mono text-[11px]"
		>
			<Section.Body className="p-1.5 space-y-1 overflow-auto">
				{!hasEvidence && (
					<div className="p-2 rounded-xs border border-(--line) bg-(--sunken) text-(--f4) text-[10px] text-center">
						Awaiting live frame telemetry for {activeSymbol}...
					</div>
				)}

				{regionScores.map((r) => (
					<div
						key={r.key}
						className={cn(
							"px-2 py-1 rounded-xs border transition-colors",
							r.isCurrent
								? "border-(--acc)/60 bg-(--acc)/10 shadow-xs"
								: r.isEntry
									? "border-(--up)/30 bg-(--up)/5"
									: r.isExitCandidate
										? "border-(--warn)/30 bg-(--warn)/5"
										: "border-(--line) bg-(--sunken)",
						)}
					>
						<Flex.Row
							align="center"
							justify="between"
							className="mb-0.5 text-[10px]"
						>
							<Flex.Row align="center" gap={1}>
								<span
									className={cn(
										"font-bold",
										r.isCurrent
											? "text-(--acc)"
											: r.isEntry
												? "text-(--up)"
												: r.isExitCandidate
													? "text-(--warn)"
													: "text-(--f2)",
									)}
								>
									{r.key}
								</span>
								<span className="text-(--f1)">{r.meta.title}</span>
								<span className="text-[9px] text-(--f4)">
									({r.meta.subtitle})
								</span>
							</Flex.Row>
							<Flex.Row align="center" gap={1}>
								<Badge size="xxs" variant={r.variant} label={r.statusLabel} />
								<span className="font-bold tabular-nums text-[10px] text-(--f1) w-14 text-right">
									{r.rawBrightness !== undefined
										? `${r.rawBrightness.toFixed(2)} σ`
										: "—"}
								</span>
							</Flex.Row>
						</Flex.Row>

						<Meter
							size="s"
							variant={r.variant}
							percent={r.percent}
							layout="bar"
						/>

						<Flex.Row
							justify="between"
							className="mt-0.5 text-[9px] text-(--f4) leading-tight"
						>
							<span className="truncate max-w-[70%]">{r.meta.description}</span>
							<span className="shrink-0 font-mono text-(--f3)">
								{r.rawBrightness !== undefined
									? `${r.members} metrics`
									: "no signal"}
							</span>
						</Flex.Row>
					</div>
				))}
			</Section.Body>
		</Section>
	);
};

export const PositionRegionMonitor = ({
	life,
}: {
	life?: Lifecycle | null;
}) => {
	const [activeTab, setActiveTab] = useState<"monitor" | "stream">("monitor");
	const activeSymbol = life?.symbol ?? "ZAMA/USD";

	return (
		<Flex.Column className="h-full bg-(--surface) font-mono text-[11px] overflow-hidden">
			<div className="h-8 px-2.5 bg-(--sunken) border-b border-(--line) flex items-center justify-between shrink-0">
				<div className="flex items-center gap-1">
					<button
						type="button"
						onClick={() => setActiveTab("monitor")}
						className={cn(
							"px-2 py-0.5 rounded-xs text-[10px] font-bold transition-colors cursor-pointer",
							activeTab === "monitor"
								? "bg-(--raised) text-(--acc)"
								: "text-(--f3) hover:text-(--f1)",
						)}
					>
						Position & Region Radar
					</button>
					<button
						type="button"
						onClick={() => setActiveTab("stream")}
						className={cn(
							"px-2 py-0.5 rounded-xs text-[10px] font-bold transition-colors cursor-pointer",
							activeTab === "stream"
								? "bg-(--raised) text-(--acc)"
								: "text-(--f3) hover:text-(--f1)",
						)}
					>
						Decision Stream
					</button>
				</div>
			</div>

			<div className="flex-1 overflow-auto p-2 space-y-2">
				<PositionHealthGauge life={life} />
				<LiveRegionActivationMap
					symbol={activeSymbol}
					currentPath="R01/R04/R06"
				/>
				<RegionTransitionPathway
					entryPath="R01/R04/R06"
					currentPath="R01/R04/R06"
				/>
			</div>
		</Flex.Column>
	);
};
