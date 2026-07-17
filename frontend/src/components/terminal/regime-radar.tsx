import { useRef } from "react";
import {
	flattenMeasurementBuffer,
	measurementsStore,
} from "#/collections/measurements";
import {
	latestByMetric,
	percentOf,
} from "#/components/terminal/measurement-view";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Panel } from "@/components/ui/panel";

/*
regimeAxes projects typed numerical measurements onto the five fields in the
backend Regime contract, then averages each field across reporting symbols.
Turbulence supplies volatility, pump trend supplies trend, and signed CVD
pressure supplies bullish, bearish, and stochastic-balance evidence.
*/
export const regimeAxes = (
	readings: typeof measurementsStore.state,
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

	for (const sourceMap of Object.values(readings.measurements)) {
		const turbulence = available.has("fluid")
			? latestByMetric(
					flattenMeasurementBuffer(sourceMap.fluid),
					"turbulent_score",
				)
			: undefined;

		if (turbulence !== undefined && turbulence.validity.state !== "invalid") {
			volatile += percentOf(turbulence) / 100;
			volatileCount += 1;
		}

		const trend = available.has("pumpdump")
			? latestByMetric(flattenMeasurementBuffer(sourceMap.pumpdump), "trend")
			: undefined;

		if (trend !== undefined && trend.validity.state !== "invalid") {
			trending += percentOf(trend) / 100;
			trendingCount += 1;
		}

		if (!available.has("cvd")) {
			continue;
		}

		const flow = flattenMeasurementBuffer(sourceMap.cvd);
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

/*
RadarPanel paints the five-axis market regime directly from typed measurement
snapshots so the right rail stays off the React render path.
*/
export const RadarPanel = ({ sources }: { sources?: string[] }) => {
	const radarFillRef = useRef<SVGPolygonElement>(null);
	const axisLabelRefs = useRef<Array<SVGTextElement | null>>([]);

	useDirectStorePaint(
		() => {
			const readings = measurementsStore.state;
			const candidates = sources ?? [
				...new Set(
					Object.values(readings.measurements).flatMap((sourceMap) =>
						Object.keys(sourceMap),
					),
				),
			];
			const axes = regimeAxes(readings, candidates);
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
				const label = axisLabelRefs.current[index];

				if (label === null || label === undefined) {
					continue;
				}

				label.textContent = axes[index]?.label ?? "—";
				label.setAttribute("x", String(110 + x * 98));
				label.setAttribute("y", String(105 + y * 98));
			}
		},
		[measurementsStore],
		[sources],
	);

	return (
		<Panel size="lg">
			<div className="mb-2 font-semibold text-(--f1) text-xs">Regime radar</div>
			<div className="mb-2 font-mono text-[9.5px] text-(--f4)">
				cross-section mean · market
			</div>
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
						ref={(element) => {
							axisLabelRefs.current[index] = element;
						}}
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
};
