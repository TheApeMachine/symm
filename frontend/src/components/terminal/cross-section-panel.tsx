import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import { usePaintStore } from "#/components/ui/paint";
import { Panel } from "#/components/ui/panel";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";
import { Metric } from "#/providers/telemetry/telemetry/metric";

const metricObj = new Metric();

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : "—";

const STATS = [
	{
		label: "med notional",
		name: "reported_volume_notional_median",
	},
	{
		label: "med depth",
		name: "executable_touch_depth_median",
	},
	{
		label: "touch depth",
		name: "executable_touch_depth",
	},
] as const;

export const CrossSectionPanel = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const store = signals["liquidity" as keyof typeof signals] || signals.cvd;

	const rootRef = usePaintStore(
		store,
		(state: any) => {
			if (!state || typeof state.getLast !== "function") return;
			const row = state.getLast();

			const metricsMap: Record<string, { raw: number; normalized: number }> =
				{};
			if (row) {
				if (Array.isArray(row.metrics)) {
					for (const m of row.metrics) {
						if (m && m.name) {
							metricsMap[m.name] = {
								raw: m.raw,
								normalized: m.normalized,
							};
						}
					}
				} else if (typeof row.metricsLength === "function") {
					for (let j = 0; j < row.metricsLength(); j++) {
						const m = row.metrics(j, metricObj);
						if (m) {
							metricsMap[m.name() ?? ""] = {
								raw: m.raw(),
								normalized: m.normalized(),
							};
						}
					}
				}
			}

			const depth = metricsMap.executable_touch_depth?.raw;
			const median = metricsMap.executable_touch_depth_median?.raw;
			const clamped =
				depth !== undefined && median !== undefined && median > 0
					? Math.min(100, Math.max(0, (depth / median) * 100))
					: 0;

			const rowAt = typeof row?.at === "function" ? row.at() : row?.at;
			const atStr = (() => {
				if (rowAt === undefined || rowAt === 0n) return "—";
				const parsed = new Date(Number(rowAt / 1000000n));
				return Number.isNaN(parsed.getTime())
					? "—"
					: parsed.toISOString().slice(11, 19);
			})();

			return {
				fields: {
					scarcity: fmt(metricsMap.scarcity_score?.raw, 3),
					symbol: focusSymbol.length === 0 ? "no focus" : focusSymbol,
					rel: fmt(metricsMap.relative_touch_depth?.raw, 3),
					at: atStr,
					norm: fmt(metricsMap.executable_touch_depth?.normalized, 2),
					[STATS[0].name]: fmt(metricsMap[STATS[0].name]?.raw, 0),
					[STATS[1].name]: fmt(metricsMap[STATS[1].name]?.raw, 0),
					[STATS[2].name]: fmt(metricsMap[STATS[2].name]?.raw, 0),
				},
				meters: {
					"depth-bar": clamped,
				},
			};
		},
		[focusSymbol],
	);

	return (
		<Panel ref={rootRef} size="lg">
			<Panel.Header
				title="Cross-section"
				meta={
					<Badge
						data-f="scarcity"
						label="—"
						variant="warning"
						size="xs"
						className="font-mono font-semibold"
					/>
				}
			/>
			<Panel.Caption>
				liquidity axes · <Typography.Span data-f="symbol" />
			</Panel.Caption>

			<Flex.Row align="center" justify="between">
				<Typography.Span variant="f2" className="text-[11px]">
					relative depth <Typography.Span data-f="rel" variant="accent" />
				</Typography.Span>
				<Typography.Span data-f="at" variant="f4" className="text-[10px]" />
			</Flex.Row>

			<Flex.Row align="center" gap={2} className="mt-2.5">
				<Typography.Label tone="f4" className="w-13.5 shrink-0 text-[9px]">
					Depth
				</Typography.Label>
				<Meter
					data-meter="depth-bar"
					layout="bar"
					size="xs"
					percent={0}
					className="flex-1"
				/>
				<Typography.Span
					data-f="norm"
					variant="f2"
					className="w-7 shrink-0 text-right text-[9px]"
				/>
			</Flex.Row>

			<Grid cols={3} gap={3} responsive={false} className="mt-3.25">
				{STATS.map((stat) => (
					<Stat
						key={stat.name}
						layout="metric"
						label={stat.label}
						value={<Typography.Span data-f={stat.name}>—</Typography.Span>}
					/>
				))}
			</Grid>
		</Panel>
	);
};
