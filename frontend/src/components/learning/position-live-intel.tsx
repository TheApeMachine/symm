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
	REGION_KEYS,
	REGIONS,
	TradeRegionScores,
} from "./position-health";

export type GateForces = {
	phase?: string;
	passed?: boolean;
	virtual?: boolean;
	reason?: string;
	ignitionHurdle?: number;
	riskTolerance?: number;
	rejectionHurdle?: number;
	netSpark?: number;
	netHold?: number;
	netRejection?: number;
	exitForce?: number;
	risk?: number;
	treeAction?: string;
	treeImpulse?: number;
	treeResistance?: number;
};

const readGate = (frame: Value | null): GateForces | null => {
	if (!frame) return null;
	const raw = frame.reading("gate") as GateForces | null;
	return raw && typeof raw === "object" ? raw : null;
};

const forceBar = (label: string, value: number | undefined, hurdle?: number) => {
	const v = value ?? 0;
	const h = hurdle ?? 0;
	const pct = Math.min(1, Math.max(0, Math.abs(v)));
	return (
		<Flex.Column gap={0.5} className="text-[10px]">
			<Flex.Row justify="between" className="text-(--f3)">
				<span>{label}</span>
				<span className="font-mono text-(--f2)">
					{v.toFixed(2)}
					{h > 0 ? ` / ${h.toFixed(2)}` : ""}
				</span>
			</Flex.Row>
			<div className="h-1.5 rounded-full bg-(--sunken) overflow-hidden">
				<div
					className={cn(
						"h-full rounded-full transition-all",
						h > 0 && v >= h ? "bg-(--up)" : "bg-(--acc)",
					)}
					style={{ width: `${100 * pct}%` }}
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
		<Section fit="content" surface="none" className="border-t border-(--line) pt-3">
			<Section.Header title="Live cognition" meta="latest tick" size="s" rule="none" />
			<Section.Body scroll={false}>
			<Flex.Column gap={2}>
				<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
					<span className="text-(--f4)">entry</span>
					<span className="text-(--f2) truncate max-w-full">{entryPath || "—"}</span>
				</Flex.Row>
				<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
					<span className="text-(--f4)">hold</span>
					<span className="text-(--acc) truncate max-w-full">{holdPath || "—"}</span>
				</Flex.Row>
				<Flex.Row gap={2} wrap="wrap" className="font-mono text-[10px]">
					<span className="text-(--f4)">exit spell</span>
					<span className="text-(--down) truncate max-w-full">{exitPath || "—"}</span>
				</Flex.Row>

				{gate && (
					<Flex.Column gap={1.5} className="rounded-xs border border-(--line) bg-(--sunken) p-2">
						<Flex.Row justify="between" className="text-[10px] uppercase tracking-wider text-(--f4)">
							<span>Gate · {gate.phase ?? "—"}</span>
							<span className={gate.passed ? "text-(--up)" : "text-(--warn)"}>
								{gate.virtual ? "virtual" : gate.passed ? "paper ok" : "blocked"}
							</span>
						</Flex.Row>
						{gate.reason ? (
							<p className="text-[10px] text-(--f3)">{gate.reason}</p>
						) : null}
						{forceBar("net spark (enter)", gate.netSpark, gate.riskTolerance)}
						{forceBar("net hold", gate.netHold, gate.ignitionHurdle)}
						{forceBar("exit force", gate.exitForce, gate.rejectionHurdle)}
						{forceBar("risk oracle", gate.risk)}
						{gate.treeAction ? (
							<p className="text-[10px] text-(--f2)">
								trie {gate.treeAction} · impulse {gate.treeImpulse?.toFixed(2)} vs
								resistance {gate.treeResistance?.toFixed(2)}
							</p>
						) : null}
					</Flex.Column>
				)}

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
