import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import type { Measurement } from "#/types/measurement";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";

/*
radarAxes are the five regime axes, laid out on a pentagon around the centre.

The unit offsets are the pentagon vertices, so the spokes and their labels sit
on the same geometry as the rings drawn behind them. Each axis names the signal
metric it reads, and sign splits the directional ones so a single flow reading
lands on either the bullish or the bearish arm rather than both.
*/
const radarAxes = [
	{
		label: "volatility",
		source: "hawkes",
		metric: "spectral_radius",
		sign: 0,
		x: 0,
		y: -1,
	},
	{
		label: "trend",
		source: "pumpdump",
		metric: "trend",
		sign: 0,
		x: 0.951,
		y: -0.309,
	},
	/*
		Cumulative delta is signed against the resting book, so a rising
		net fraction is flow lifting offers into a market that is selling it:
		the bearish arm takes the positive readings.
	*/
	{
		label: "bullish",
		source: "cvd",
		metric: "net_fraction",
		sign: -1,
		x: 0.588,
		y: 0.809,
	},
	{
		label: "bearish",
		source: "cvd",
		metric: "net_fraction",
		sign: 1,
		x: -0.588,
		y: 0.809,
	},
	{
		label: "chop",
		source: "cvd",
		metric: "balance",
		sign: 0,
		x: -0.951,
		y: -0.309,
	},
];

/*
regimeAxes projects the current measurement batch onto the regime axes.

Only normalized readings are plotted, because the axes share one radius and a
raw reading carries whatever units its signal happens to use. A source that has
not reported reads as zero rather than collapsing the polygon.
*/
export const regimeAxes = (
	measurements: Measurement[],
	sources: string[],
): { label: string; value: number }[] => {
	const active = new Set(sources);

	return radarAxes.map((axis) => {
		if (!active.has(axis.source)) {
			return { label: axis.label, value: 0 };
		}

		const reading = measurements.find(
			(measurement) =>
				measurement.source === axis.source &&
				measurement.metrics?.[axis.metric]?.normalized != null,
		);

		const value = reading?.metrics?.[axis.metric]?.normalized ?? 0;

		if (axis.sign > 0) {
			return { label: axis.label, value: Math.max(0, value) };
		}

		if (axis.sign < 0) {
			return { label: axis.label, value: Math.max(0, -value) };
		}

		return { label: axis.label, value };
	});
};

/*
RadarPanel is the regime-radar shell.

The rings and spokes are fixed geometry, so only the reading itself is painted:
the filled polygon takes its points from the measurement cross-section, and
each axis takes its value from the matching key.
*/
export const RadarPanel = () => (
	<Component registerKey="measurements">
		{({ ref, className }) => (
			<div ref={ref} className={cn("flex h-full flex-col", className)}>
				<Panel size="lg">
					<Flex className="mb-2 font-semibold text-(--f1) text-xs">
						Regime radar
					</Flex>
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

						{radarAxes.map((axis) => (
							<line
								key={`spoke:${axis.label}`}
								x1="110"
								y1="105"
								x2={110 + axis.x * 84}
								y2={105 + axis.y * 84}
								stroke="#2b251e"
							/>
						))}

						<polygon
							data-radar="reading"
							fill="rgba(232,163,61,0.22)"
							stroke="#e8a33d"
							strokeWidth="1.6"
						/>

						{radarAxes.map((axis) => (
							<text
								key={`label:${axis.label}`}
								x={110 + axis.x * 98}
								y={105 + axis.y * 98}
								textAnchor="middle"
								fontSize="9"
								fill="#938a7e"
							>
								{axis.label}
							</text>
						))}
					</svg>
				</Panel>
			</div>
		)}
	</Component>
);
