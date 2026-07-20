import { createRef } from "react";
import type { Measurement } from "#/types/measurement";
import { backendMeasurementSources } from "#/components/terminal/measurement-sources";
import {
	latestByMetric,
	percentOf,
} from "#/components/terminal/measurement-view";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";

/*
regimeAxes projects typed numerical measurements onto the five fields in the
backend Regime contract, then averages each field across reporting symbols.
*/
export const regimeAxes = (
	measurements: Measurement[],
	sources: string[],
): Array<{ label: string; value: number }> => {
	const available = new Set(sources);
	let volatile = 0;
	let volatileCount = 0;
	let trending = 0;
	let trendingCount = 0;
	let bullish = 0;
	let bearish = 0;
	let directionalCount = 0;
	let choppy = 0;
	let choppyCount = 0;
	const symbols = [
		...new Set(measurements.map((row) => row.symbol).filter(Boolean)),
	];

	for (const symbol of symbols) {
		const symbolRows = measurements.filter((row) => row.symbol === symbol);
		const turbulence = available.has("fluid")
			? latestByMetric(
					symbolRows.filter((row) => row.source === "fluid"),
					"turbulent_score",
				)
			: undefined;

		if (turbulence !== undefined && turbulence.validity.state !== "invalid") {
			volatile += percentOf(turbulence) / 100;
			volatileCount += 1;
		}

		const trend = available.has("pumpdump")
			? latestByMetric(
					symbolRows.filter((row) => row.source === "pumpdump"),
					"trend",
				)
			: undefined;

		if (trend !== undefined && trend.validity.state !== "invalid") {
			trending += percentOf(trend) / 100;
			trendingCount += 1;
		}

		if (!available.has("cvd")) {
			continue;
		}

		const flow = symbolRows.filter((row) => row.source === "cvd");
		const net = latestByMetric(flow, "net");
		const netFraction = latestByMetric(flow, "net_fraction");
		const balance = latestByMetric(flow, "balance");

		if (
			net !== undefined &&
			netFraction !== undefined &&
			net.validity.state !== "invalid" &&
			netFraction.validity.state !== "invalid"
		) {
			const pressure = percentOf(netFraction) / 100;
			bullish += net.raw > 0 ? pressure : 0;
			bearish += net.raw < 0 ? pressure : 0;
			directionalCount += 1;
		}

		if (balance !== undefined && balance.validity.state !== "invalid") {
			choppy += percentOf(balance) / 100;
			choppyCount += 1;
		}
	}

	return [
		{
			label: "volatility",
			value: volatileCount > 0 ? volatile / volatileCount : 0,
		},
		{ label: "trend", value: trendingCount > 0 ? trending / trendingCount : 0 },
		{
			label: "bullish",
			value: directionalCount > 0 ? bullish / directionalCount : 0,
		},
		{
			label: "bearish",
			value: directionalCount > 0 ? bearish / directionalCount : 0,
		},
		{ label: "chop", value: choppyCount > 0 ? choppy / choppyCount : 0 },
	];
};

const RADAR_UNITS: Array<[number, number]> = [
	[0, -1],
	[0.951, -0.309],
	[0.588, 0.809],
	[-0.588, 0.809],
	[-0.951, -0.309],
];

const radarFillRef = createRef<SVGPolygonElement>();
const axisLabelRefs = RADAR_UNITS.map(() => createRef<SVGTextElement>());

/*
paintRegimeRadar paints the five-axis market regime from the current DRAW
measurements batch into the RadarPanel shell.
*/
export const paintRegimeRadar = (value: unknown, _focusSymbol: string) => {
	const measurements = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
	const candidates = backendMeasurementSources(measurements);
	const axes = regimeAxes(measurements, candidates);
	const points = RADAR_UNITS.map(
		([x, y], index) =>
			`${110 + x * 84 * (axes[index]?.value ?? 0)},${
				105 + y * 84 * (axes[index]?.value ?? 0)
			}`,
	).join(" ");

	if (radarFillRef.current !== null) {
		radarFillRef.current.setAttribute("points", points);
	}

	for (const [index, [x, y]] of RADAR_UNITS.entries()) {
		const label = axisLabelRefs[index]?.current;

		if (label === null || label === undefined) {
			continue;
		}

		label.textContent = axes[index]?.label ?? "—";
		label.setAttribute("x", String(110 + x * 98));
		label.setAttribute("y", String(105 + y * 98));
	}
};

/*
RadarPanel is the static regime-radar shell. DRAW paints via paintRegimeRadar.
*/
export const RadarPanel = () => (
	<Panel size="lg">
		<Flex className="mb-2 font-semibold text-(--f1) text-xs">Regime radar</Flex>
		<Flex className="mb-2 font-mono text-[9.5px] text-(--f4)">
			cross-section mean · market
		</Flex>
		<svg viewBox="0 0 220 210" className="block w-full">
			<title>Regime radar</title>
			<polygon
				points="110,21 190,79 159,173 61,173 30,79"
				fill="none"
				stroke="#3a342b"
			/>
			<polygon
				points="110,49 163,87 142,154 78,154 57,87"
				fill="none"
				stroke="#2b251e"
			/>
			<polygon
				points="110,77 137,94 126,134 94,134 83,94"
				fill="none"
				stroke="#2b251e"
			/>
			{RADAR_UNITS.map(([x, y]) => (
				<line
					key={`${x}:${y}`}
					x1="110"
					y1="105"
					x2={110 + x * 84}
					y2={105 + y * 84}
					stroke="#2b251e"
				/>
			))}
			<polygon
				ref={radarFillRef}
				fill="rgba(232,163,61,0.22)"
				stroke="#e8a33d"
				strokeWidth="1.6"
			/>
			{RADAR_UNITS.map(([x, y], index) => (
				<text
					key={`${x}:${y}`}
					ref={axisLabelRefs[index]}
					x={110 + x * 98}
					y={105 + y * 98}
					textAnchor="middle"
					fontSize="9"
					fill="#938a7e"
				/>
			))}
		</svg>
	</Panel>
);
