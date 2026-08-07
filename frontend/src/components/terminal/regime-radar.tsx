import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
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
		A signed reading cannot be split across two arms by a data attribute, and
		the cumulative-delta kernel already publishes the two directions as
		readings of their own: drive is flow pushing through the book, starvation
		is flow failing to. Pointing both arms at one signed number would have made
		them mirror each other rather than disagree.
	*/
	{
		label: "drive",
		source: "cvd",
		metric: "drive",
		sign: 0,
		x: 0.588,
		y: 0.809,
	},
	{
		label: "starved",
		source: "cvd",
		metric: "starvation",
		sign: 0,
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
RadarPanel is the regime-radar shell.

The rings, spokes and labels are fixed geometry. Each axis is then its own
painted arm: the group carries the reading as a custom property and is scaled
about the centre by it, so an axis reaches its vertex at full strength and
collapses toward the middle at none. A single filled polygon would have needed
its points recomputed in the browser, which is neither something a data
attribute can express nor something the browser should be deciding.

Measurements are published for the focused symbol, so this is that symbol's
regime rather than a market-wide mean.
*/
export const RadarPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="measurements">
			{({ ref, className }) => (
				<div ref={ref} className={cn("flex h-full flex-col", className)}>
					<Panel size="lg">
						<Flex className="mb-2 font-semibold text-(--f1) text-xs">
							Regime radar
						</Flex>
						<Flex className="mb-2 font-mono text-[9.5px] text-(--f4)">
							{focusSymbol} · normalized axes
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

							{radarAxes.map((axis) => (
								<g
									key={`arm:${axis.label}`}
									data-scope="source,symbol"
									data-filter={`${axis.source},${focusSymbol}`}
									style={{
										transform: "scale(clamp(0, var(--axis, 0), 1))",
										transformOrigin: "110px 105px",
									}}
								>
									<line
										data-set={`metrics.${axis.metric}.normalized`}
										data-target="style.--axis"
										x1="110"
										y1="105"
										x2={110 + axis.x * 84}
										y2={105 + axis.y * 84}
										stroke="#e8a33d"
										strokeWidth="1.6"
									/>
									<circle
										cx={110 + axis.x * 84}
										cy={105 + axis.y * 84}
										r="2.6"
										fill="#e8a33d"
									/>
								</g>
							))}

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
};
