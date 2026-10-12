import { signals } from "#/collections/app";
import {
	Component,
	DistanceBar,
	formatValue,
	Funnel,
	Gauge,
	type GaugeTone,
	metricOf,
	metricsWithPrefix,
	RadialHistory,
	sectorHue,
	TokenRibbon,
	Trail,
	useMeasurements,
} from "#/components/ui";
import { cn } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { useSelector } from "@tanstack/react-store";

/*
Open positions, drawn from the Training Measurement's own Metrics
(position_*, region_brightness:*, path_token:*, match_*). Nothing here is
computed that the backend did not send; missing Metrics show as waiting.
*/

const REGIONS = 12;
const regionLabel = (s: number) => `R${String(s).padStart(2, "0")}`;
const regionColor = (s: number) => sectorHue(s, REGIONS);

const heldSymbols = (
	state: Record<string, { getLast(): MeasurementT | undefined }>,
) =>
	Object.entries(state)
		.filter(([, ring]) => metricOf(ring?.getLast(), "position_open") === 1)
		.map(([symbol]) => symbol)
		.sort()
		.join("|");

const duration = (s?: number) => {
	if (s === undefined) return "—";
	const m = Math.floor(s / 60);
	const h = Math.floor(m / 60);
	return h > 0
		? `${h}h ${m % 60}m`
		: m > 0
			? `${m}m ${Math.floor(s % 60)}s`
			: `${Math.floor(s)}s`;
};

const tokensOf = (m?: MeasurementT): number[][] => {
	const tokens: number[][] = [];
	for (const [key, region] of metricsWithPrefix(m, "path_token:")) {
		const [i, j] = key.split(":").map(Number);
		tokens[i] ??= [];
		tokens[i][j] = region;
	}
	return tokens.filter(Boolean);
};

const regionRow = (m: MeasurementT): Array<number | undefined> => {
	const row: Array<number | undefined> = new Array(REGIONS).fill(undefined);
	for (const [key, v] of metricsWithPrefix(m, "region_brightness:R"))
		row[Number(key)] = v;
	return row;
};

const PositionCard = ({ symbol }: { symbol: string }) => {
	const history = useMeasurements(signals.training, symbol, 90);
	const latest = history[history.length - 1];
	const m = (name: string) => metricOf(latest, name);

	const entry = m("position_entry");
	const bid = m("position_bid");
	const qty = m("position_qty");
	const cost = m("position_cost");
	const value = m("position_value");
	const vwap = m("position_exit_vwap");
	const pnlPct = m("position_pnl_pct");
	const room = m("position_exit_room");
	const exiting = m("position_exiting") === 1;
	const breakEven =
		cost !== undefined && value !== undefined && vwap && qty
			? cost / (qty * (value / (vwap * qty)))
			: undefined;

	const pnlTone: GaugeTone =
		pnlPct === undefined ? "muted" : pnlPct >= 0 ? "up" : "down";
	const roomTone: GaugeTone =
		room === undefined ? "muted" : room < 1 ? "down" : room < 3 ? "warn" : "up";
	const pnlTrail = history
		.map((h) => metricOf(h, "position_pnl_pct"))
		.filter((v): v is number => v !== undefined);
	const deviations = history.flatMap((h) => {
		const b = metricOf(h, "position_bid");
		const e = metricOf(h, "position_entry");
		return b !== undefined && e !== undefined ? [Math.abs(b - e)] : [];
	});

	const rows = history
		.filter(
			(h) =>
				metricOf(h, "region_brightness:R00") !== undefined ||
				metricsWithPrefix(h, "region_brightness:").size > 0,
		)
		.slice(-24)
		.map(regionRow);
	const peak = Math.max(
		1,
		...rows.flat().filter((v): v is number => v !== undefined),
	);
	const scaled = rows.map((r) =>
		r.map((v) => (v === undefined ? v : v / peak)),
	);

	const withTokens = [...history]
		.reverse()
		.find((h) => metricOf(h, "token_depth") !== undefined);
	const tokens = tokensOf(withTokens);
	const capacity = metricOf(withTokens, "token_capacity");
	const strength = metricOf(withTokens, "token_strength");
	const threshold = metricOf(withTokens, "token_threshold");
	const stored = metricOf(withTokens, "stored_paths") ?? 0;
	const stages = tokens.map((_, i) => {
		const k = String(i + 1).padStart(2, "0");
		const total = metricOf(withTokens, `match_candidates:${k}`) ?? 0;
		const enter = metricOf(withTokens, `match_enter:${k}`) ?? 0;
		const exit = metricOf(withTokens, `match_exit:${k}`) ?? 0;
		return {
			label: String(i + 1),
			segments: [
				{ value: enter, color: "var(--up)", label: "say enter" },
				{ value: exit, color: "var(--down)", label: "say exit" },
				{
					value: Math.max(0, total - enter - exit),
					color: "var(--f4)",
					label: "other",
				},
			],
		};
	});
	const lastStage = stages[stages.length - 1]?.segments;
	const verdict = !lastStage
		? "waiting for tokens"
		: lastStage.every((s) => s.value === 0)
			? "no stored path fits this sequence"
			: lastStage[1].value > 0 && lastStage[0].value === 0
				? "fitting paths point to EXIT"
				: lastStage[0].value > 0 && lastStage[1].value === 0
					? "fitting paths point to ENTER"
					: "fitting paths disagree";

	return (
		<Component store={signals.training} symbol={symbol}>
			<div
				className={cn(
					"rounded-xs border bg-(--surface) p-3 flex flex-col gap-3 transition-shadow",
					exiting && "animate-pulse",
				)}
				style={{
					borderColor: `color-mix(in srgb, var(--${pnlTone === "muted" ? "line" : pnlTone}) 60%, var(--line))`,
					boxShadow:
						pnlTone === "muted" ? undefined : `0 0 12px -8px var(--${pnlTone})`,
				}}
			>
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-2">
						<span
							className="w-2.5 h-2.5 rounded-full"
							style={{
								background: `var(--${pnlTone === "muted" ? "f4" : pnlTone})`,
							}}
						/>
						<span className="text-(--f1) font-bold text-sm">{symbol}</span>
						{exiting && (
							<span className="text-[10px] px-1.5 rounded bg-(--down)/20 text-(--down)">
								selling…
							</span>
						)}
					</div>
					<span className="text-[10px] text-(--f3)">
						held {duration(m("position_held_s"))}
					</span>
				</div>

				<div className="grid grid-cols-[auto_1fr] gap-4 items-center">
					<div className="flex gap-1">
						<Gauge
							value={
								pnlPct === undefined
									? undefined
									: 0.5 + Math.max(-0.5, Math.min(0.5, pnlPct * 5))
							}
							tone={pnlTone}
							ticks={[0.5]}
							label={
								pnlPct === undefined ? undefined : (
									<span style={{ color: `var(--${pnlTone})` }}>
										{formatValue(pnlPct, "pct")}
									</span>
								)
							}
							caption="if sold now"
						/>
						<Gauge
							value={
								room === undefined
									? undefined
									: Math.max(
											0,
											Math.min(
												1,
												Math.log10(Math.max(room, 1e-3) * 10) / Math.log10(200),
											),
										)
							}
							tone={roomTone}
							ticks={[Math.log10(10) / Math.log10(200)]}
							label={
								room === undefined
									? undefined
									: `${room >= 100 ? Math.round(room) : room.toFixed(1)}×`
							}
							caption="exit room"
						/>
					</div>
					<div className="flex flex-col gap-2 min-w-0">
						<DistanceBar
							origin={entry}
							value={bid}
							reach={
								deviations.length ? Math.max(...deviations) * 1.2 : undefined
							}
							markers={[
								...(vwap !== undefined
									? [
											{
												value: vwap,
												label: "average price selling everything",
												color: "var(--info)",
											},
										]
									: []),
								...(breakEven !== undefined
									? [
											{
												value: breakEven,
												label: "break-even after fees",
												color: "var(--warn)",
											},
										]
									: []),
							]}
							leftLabel="below entry"
							rightLabel="above entry"
						/>
						<Trail values={pnlTrail} width={240} height={40} />
					</div>
				</div>

				<div className="grid grid-cols-[auto_1fr] gap-4 items-start border-t border-(--line) pt-3">
					<div className="flex flex-col items-center">
						<RadialHistory
							sectors={REGIONS}
							rows={scaled}
							highlight={tokens[tokens.length - 1] ?? []}
							labels={regionLabel}
							size={170}
						/>
						<span className="text-[9px] text-(--f4)">
							grid regions · outer ring = now
						</span>
					</div>
					<div className="flex flex-col gap-3 min-w-0">
						<div>
							<div className="text-[10px] text-(--f3) mb-1">token sequence</div>
							<TokenRibbon
								tokens={tokens}
								capacity={capacity}
								color={regionColor}
								label={regionLabel}
							/>
						</div>
						<div className="flex items-center gap-2">
							<Gauge
								size={56}
								sweep={180}
								value={
									strength === undefined || !threshold
										? undefined
										: Math.min(1, strength / (2 * threshold))
								}
								tone={
									strength !== undefined &&
									threshold !== undefined &&
									strength >= threshold
										? "acc"
										: "muted"
								}
								ticks={[0.5]}
							/>
							<span className="text-[10px] text-(--f3) leading-tight">
								token strength
								<br />
								<span className="text-(--f4)">tick = needed to advance</span>
							</span>
						</div>
						<div>
							<div className="text-[10px] text-(--f3) mb-1">
								stored paths still fitting ({stored} total)
							</div>
							<Funnel total={stored} stages={stages} />
							<div className="text-[10px] text-(--f2) mt-1">{verdict}</div>
						</div>
					</div>
				</div>
			</div>
		</Component>
	);
};

export const OpenPositions = () => {
	const held = useSelector(signals.training, heldSymbols);
	const symbols = held ? held.split("|") : [];

	if (symbols.length === 0) {
		return (
			<div className="flex-1 flex flex-col items-center justify-center gap-2 text-(--f3)">
				<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
				<span className="text-sm">No open positions</span>
				<span className="text-[10px] text-(--f4)">
					cards appear here as soon as the desk holds a symbol
				</span>
			</div>
		);
	}

	return (
		<div className="flex-1 min-h-0 overflow-auto p-3 grid gap-3 grid-cols-[repeat(auto-fill,minmax(440px,1fr))] content-start">
			{symbols.map((symbol) => (
				<PositionCard key={symbol} symbol={symbol} />
			))}
		</div>
	);
};
