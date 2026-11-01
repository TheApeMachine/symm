import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { signals } from "#/collections/app";
import type { Decision, Region } from "#/collections/envelope";
import {
	gateLabStore,
	ORACLE_GATES,
	type OracleGate,
	type OracleOverride,
} from "#/collections/gate-lab";
import { GateLab, GateLabToggle } from "#/components/dashboard/gate-lab";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { formatBlockedReason } from "#/lib/trigger-labels";
import { cn } from "#/lib/utils";

type GateSide = "veto" | "approve" | "dark";

type DecisionRow = {
	id: string;
	symbol: string;
	action: string;
	confidence: number;
	reason: string;
	gate?: string;
	path?: string;
};

const decisionReason = (decision: Decision): string => {
	const reason = formatBlockedReason(decision.reason || "");
	const gate = decision.gate ? `${decision.gate}: ` : "";
	const path = decision.path ? ` · ${decision.path}` : "";
	const virtual = decision.virtual ? " · virtual" : "";

	return `${gate}${reason}${path}${virtual}`;
};

const decisionsFromLive = (live: Record<string, Decision>): DecisionRow[] => {
	const rows: DecisionRow[] = [];

	for (const decision of Object.values(live)) {
		if (!decision?.symbol || !decision.action) continue;

		rows.push({
			id: `${decision.symbol}-${decision.action}-${decision.reason}`,
			symbol: decision.symbol,
			action: decision.action,
			confidence: decision.confidence ?? 0,
			reason: decisionReason(decision),
			gate: decision.gate,
			path: decision.path,
		});
	}

	return rows.sort((left, right) => {
		const rank = (action: string) =>
			action === "blocked"
				? 0
				: action === "enter" || action === "exit"
					? 1
					: 2;
		const byAction = rank(left.action) - rank(right.action);
		if (byAction !== 0) return byAction;
		return left.symbol.localeCompare(right.symbol);
	});
};

const brightnessAt = (rows: Region[] | undefined, key: number): number => {
	if (!rows) return 0;

	for (const row of rows) {
		if (row?.key === key) {
			return typeof row.brightness === "number" &&
				Number.isFinite(row.brightness)
				? row.brightness
				: 0;
		}
	}

	return 0;
};

const pathKeys = (path: string | undefined): Set<number> => {
	const out = new Set<number>();
	if (!path) return out;

	for (const part of path.split("/")) {
		const match = /^R(\d{2})$/.exec(part.trim());
		if (!match) continue;
		out.add(Number(match[1]));
	}

	return out;
};

const gateSide = (
	gate: OracleGate,
	rows: Region[] | undefined,
	lit: Set<number>,
	override: OracleOverride | undefined,
): GateSide => {
	if (override === "veto" || override === "approve") {
		return override;
	}

	const veto = brightnessAt(rows, gate.veto);
	const approve = brightnessAt(rows, gate.approve);

	if (veto > 0 || approve > 0) {
		return veto >= approve ? "veto" : "approve";
	}

	if (lit.has(gate.veto)) return "veto";
	if (lit.has(gate.approve)) return "approve";
	return "dark";
};

const GateLamps = ({
	symbol,
	path,
	regions,
	overrides,
}: {
	symbol: string;
	path?: string;
	regions: Record<string, Region[]>;
	overrides: Record<string, OracleOverride>;
}) => {
	const rows =
		regions[symbol] ?? regions[symbol.replace(/\//g, "-")] ?? undefined;
	const lit = pathKeys(path);

	return (
		<Flex.Row
			align="center"
			gap={1}
			className="col-span-2 mt-1 min-w-0"
			aria-label={`${symbol} oracle gates`}
		>
			{ORACLE_GATES.map((gate) => {
				const side = gateSide(gate, rows, lit, overrides[gate.id]);
				const forced = overrides[gate.id] && overrides[gate.id] !== "auto";
				const state =
					side === "dark" ? "dark" : `${side}${forced ? " · forced" : ""}`;

				return (
					<span
						key={gate.id}
						title={`${gate.label} (${state}). ${gate.hint}`}
						data-oracle-gate={gate.id}
						data-oracle-side={side}
						data-oracle-forced={forced ? "true" : "false"}
						className={cn(
							"inline-block size-1.5 shrink-0 rounded-[1px]",
							side === "approve" && "bg-(--up)",
							side === "veto" && "bg-(--down)",
							side === "dark" && "bg-(--line2)",
							forced && "ring-1 ring-(--acc)/70",
						)}
					/>
				);
			})}
			<Typography.Span className="ml-1 truncate text-[8px] tracking-[0.04em] text-(--f4) uppercase">
				{ORACLE_GATES.map((gate) => gate.id).join(" ")}
			</Typography.Span>
		</Flex.Row>
	);
};

export const Decisions = () => {
	const live = useSelector(signals.decisions, (state) => state);
	const regions = useSelector(signals.regions, (state) => state);
	const overrides = useSelector(gateLabStore, (state) => state.overrides);
	const decisions = useMemo(() => decisionsFromLive(live), [live]);

	const inspectDecision = (symbol: string) => {
		setDecisionsScopeSymbol(symbol);
		setDecisionsPendingFocus(symbol);
	};

	return (
		<Flex.Column className="h-full min-h-0 gap-0">
			<Flex.Row
				align="baseline"
				justify="between"
				padding={2}
				className="border-(--line) border-b"
			>
				<Typography.Span semibold uppercase tracking="0.13em">
					DECISIONS
				</Typography.Span>
				<Flex.Row align="center" gap={2}>
					<Typography.Span className="text-[9px] text-(--f4)">
						{decisions.length ? `${decisions.length} live` : "idle"}
					</Typography.Span>
					<GateLabToggle />
				</Flex.Row>
			</Flex.Row>
			<GateLab />
			<List className="min-h-0 flex-1 gap-1 overflow-auto p-2">
				{decisions.length === 0 ? (
					<List.Item className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 px-2.5 py-1.5 font-mono text-[11px] text-(--f4)">
						waiting for backend decision frames
					</List.Item>
				) : (
					decisions.map((dec) => (
						<List.Item
							key={dec.id}
							className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)] hover:bg-(--raised)"
							data-decision-card="true"
							data-decision-id={dec.id}
							data-decision-action={dec.action}
							onClick={() => inspectDecision(dec.symbol)}
							title={dec.reason}
						>
							<Typography.Span className="truncate font-semibold text-[11px] text-(--f1)">
								{dec.symbol}
							</Typography.Span>
							<Flex.Row className="items-center gap-2">
								{dec.confidence > 0 ? (
									<Typography.Span className="text-[8.5px] text-(--f4)">
										conf=
										<span className="tabular-nums text-(--f2)">
											{dec.confidence.toFixed(4)}
										</span>
									</Typography.Span>
								) : null}
								<Typography.Span
									className={
										dec.action === "blocked"
											? "rounded-xs border border-(--warn)/40 px-1.5 py-px text-[8.5px] uppercase text-(--warn)"
											: "rounded-xs border border-(--line) px-1.5 py-px text-[8.5px] uppercase"
									}
								>
									{dec.action}
								</Typography.Span>
							</Flex.Row>
							<GateLamps
								symbol={dec.symbol}
								path={dec.path}
								regions={regions}
								overrides={overrides}
							/>
							<Typography.Span className="col-span-2 mt-0.5 line-clamp-2 min-w-0 wrap-break-word text-[9px] leading-tight text-(--f4)">
								{dec.reason}
							</Typography.Span>
						</List.Item>
					))
				)}
			</List>
		</Flex.Column>
	);
};
