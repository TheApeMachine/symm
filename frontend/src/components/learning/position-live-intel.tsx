import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { signals } from "#/collections/app";
import { sequiturStore } from "#/collections/sequitur";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import type { Lifecycle } from "#/lib/lifecycle";
import { cn } from "#/lib/utils";
import type { Value } from "#/types/value";
import {
	formatGateBadge,
	formatTrieLine,
	type GateForces,
	gateBadgeClassName,
} from "./gate-labels";
import { REGION_KEYS, REGIONS, TradeRegionScores } from "./position-health";

export type { GateForces };

const EMPTY_GATE: GateForces = {
	phase: "hold",
	passed: false,
	ignitionHurdle: 1,
	riskTolerance: 1,
	rejectionHurdle: 1,
	netSpark: 0,
	netHold: 0,
	exitForce: 0,
	risk: 0,
	treeImpulse: 0,
	treeResistance: 1,
};

const readGate = (frame: Value | null): GateForces => {
	if (!frame) return EMPTY_GATE;
	const raw = frame.reading("gate") as GateForces | null;
	return raw && typeof raw === "object"
		? { ...EMPTY_GATE, ...raw }
		: EMPTY_GATE;
};

const forceBar = (
	label: string,
	value: number | undefined,
	opts?: { hurdle?: number; compareHurdle?: boolean },
) => {
	const v = value ?? 0;
	const h = opts?.hurdle ?? 0;
	const compare = opts?.compareHurdle ?? h > 0;
	const fillPct =
		h > 0 && compare
			? Math.min(1, Math.max(0, v / h))
			: Math.min(1, Math.max(0, Math.abs(v)));
	const overHurdle = compare && h > 0 && v >= h;
	return (
		<Flex.Column gap={0.5} className="text-[10px]">
			<Flex.Row justify="between" className="text-(--f3)">
				<span>{label}</span>
				<span className="font-mono text-(--f2)">
					{v.toFixed(2)}
					{h > 0 ? ` · hurdle ${h.toFixed(2)}` : ""}
				</span>
			</Flex.Row>
			<div className="h-1.5 rounded-full bg-(--sunken) overflow-hidden">
				<div
					className={cn(
						"h-full rounded-full transition-all",
						overHurdle ? "bg-(--up)" : "bg-(--acc)",
					)}
					style={{ width: `${100 * fillPct}%` }}
				/>
			</div>
		</Flex.Column>
	);
};

/*
PositionLiveIntel shows the live training step for one open lot: which regions
lit, gate vectors, trie action, and the three path keys (entry / hold / exit).
*/
export const PositionLiveIntel = ({
	symbol,
	row,
}: {
	symbol: string;
	row: Record<string, unknown>;
}) => {
	const trainingFrame = useSelector(signals.training, (state) => state[symbol]);
	const sequitur = useSelector(
		sequiturStore,
		(state) => state[symbol]?.latest ?? null,
	);

	const life = useMemo(
		(): Lifecycle =>
			({
				symbol,
				book: row.virtual === true ? "virtual" : "paper",
				status: "open",
			}) as Lifecycle,
		[symbol, row.virtual],
	);

	const gate = readGate(trainingFrame ?? null);
	const gateLive = Boolean(trainingFrame?.reading("gate"));
	const enterBlocked = Boolean(trainingFrame?.reading("blocked"));
	const blockedReason =
		(trainingFrame?.reading("blockedReason") as string | undefined) ??
		undefined;
	const phase = gate.phase ?? "hold";
	const positionBook = life.book === "virtual" ? "virtual" : "paper";
	const gateBadge = formatGateBadge({
		gateLive,
		phase,
		gate,
		blocked: enterBlocked,
		positionBook,
	});
	const exitArm =
		gate.treeAction === "exit" &&
		(gate.treeImpulse ?? 0) >= (gate.treeResistance ?? 0);
	const exitDecision = phase === "exit";
	const active =
		sequitur?.active ??
		(trainingFrame?.reading("active") as Array<{
			key: number;
			brightness: number;
		}>) ??
		[];

	const entryPath = String(row.entry_path ?? "");
	const holdPath = String(row.hold_path ?? "");
	const exitPath = String(row.exit_path ?? "");

	return (
		<Section
			fit="content"
			surface="none"
			className="border-t border-(--line) pt-3"
		>
			<Section.Header
				title="Live cognition"
				meta="latest tick"
				size="s"
				rule="none"
			/>
			<Section.Body scroll={false}>
				<Flex.Column gap={2}>
					<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
						<span className="text-(--f4)">entry</span>
						<span className="text-(--f2) truncate max-w-full">
							{entryPath || "—"}
						</span>
					</Flex.Row>
					<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
						<span className="text-(--f4)">hold</span>
						<span className="text-(--acc) truncate max-w-full">
							{holdPath || "—"}
						</span>
					</Flex.Row>
					<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
						<span className="text-(--f4)">exit spell</span>
						<span className="text-(--down) truncate max-w-full">
							{exitPath || "—"}
						</span>
					</Flex.Row>

					<Flex.Column
						gap={1.5}
						className="rounded-xs border border-(--line) bg-(--sunken) p-2"
					>
						<Flex.Row
							justify="between"
							className="text-[10px] uppercase tracking-wider text-(--f4)"
						>
							<span>Gate · {gateLive ? phase : "waiting"}</span>
							<span className={gateBadgeClassName(gateBadge.tone)}>
								{gateBadge.label}
							</span>
						</Flex.Row>
						<p className="text-[9px] leading-snug text-(--f4)">
							{gateBadge.hint}
						</p>
						<p
							className={cn(
								"min-h-8 text-[10px] leading-snug",
								gate.reason || blockedReason
									? gate.reason
										? "text-(--f3)"
										: "text-(--warn)"
									: "text-(--f4)",
							)}
						>
							{blockedReason ?? gate.reason ?? "—"}
						</p>
						{forceBar("net spark (enter)", gate.netSpark, {
							hurdle: gate.riskTolerance,
						})}
						{forceBar("net hold", gate.netHold, {
							hurdle: gate.ignitionHurdle,
						})}
						{forceBar(
							exitDecision ? "exit force" : "exit band pressure",
							gate.exitForce,
							{
								hurdle: gate.rejectionHurdle,
								compareHurdle: exitDecision,
							},
						)}
						{forceBar("risk oracle", gate.risk)}
						<p className="min-h-11 text-[9px] leading-snug text-(--f4)">
							Exit band pressure is R03–R08 on this tick. A cognitive exit still
							needs the exit trie (impulse ≥ resistance), close-now profit, and
							the gate hurdle.
							{exitArm
								? " Exit arm is hot this tick."
								: " Exit trie is still hold/noop."}
						</p>
						<p className="text-[10px] text-(--f2)">
							{formatTrieLine(gate, phase)}
						</p>
					</Flex.Column>

					<div className="grid grid-cols-5 gap-0.5">
						{REGION_KEYS.map((key) => {
							const regionKey = Number(key.slice(1));
							const hit = active.find((r) => r.key === regionKey);
							const b = hit?.brightness ?? 0;
							const meta = REGIONS[key];
							return (
								<div
									key={key}
									title={meta?.title}
									className={cn(
										"rounded-xs border px-0.5 py-1 text-center text-[8px] font-mono",
										b >= 0.5
											? "border-(--acc) bg-(--acc)/15 text-(--acc)"
											: b > 0
												? "border-(--line) bg-(--surface) text-(--f3)"
												: "border-transparent text-(--f4)",
									)}
								>
									<div>{key}</div>
									<div>{b > 0 ? b.toFixed(2) : "·"}</div>
								</div>
							);
						})}
					</div>

					<TradeRegionScores life={life} />
				</Flex.Column>
			</Section.Body>
		</Section>
	);
};
