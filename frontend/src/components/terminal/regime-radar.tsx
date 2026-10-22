import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { atoms, signalFor, signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";

const radarAxes = [
	{
		label: "volatility",
		source: "hawkes",
		metric: "branching_spectral_radius",
		key: "branching_spectral_radius",
		x: 0,
		y: -1,
	},
	{
		label: "trend",
		source: "pumpdump",
		metric: "positive_midpoint_return",
		key: "positive_midpoint_return",
		x: 0.951,
		y: -0.309,
	},
	{
		label: "drive",
		source: "cvd",
		metric: "signed_net_fraction",
		key: "signed_net_fraction",
		x: 0.588,
		y: 0.809,
	},
	{
		label: "starved",
		source: "pumpdump",
		metric: "trade_rate",
		key: "trade_rate",
		x: -0.588,
		y: 0.809,
	},
	{
		label: "chop",
		source: "depthflow",
		metric: "book_imbalance",
		key: "book_imbalance",
		x: -0.951,
		y: -0.309,
	},
];

export const RadarPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		/*
		The kernels publish into the workspace tick as one frame: the
		aggregate is the only store, and signalFor pulls one kernel's
		layers for the focus symbol off it.
		*/
		const apply = () => {
			if (!root.current) return;

			for (const axis of radarAxes) {
				// signalFor maps each metric key to its scalar array.
				const payloads = signalFor(axis.source, focusSymbol);
				const payload = payloads[axis.key];
				const found = Array.isArray(payload) ? payload[0] : payload;
				const value =
					typeof found === "number" && Number.isFinite(found)
						? found
						: 0;

				const arm = root.current.querySelector<SVGElement>(
					`[data-axis="${axis.label}"]`,
				);
				if (arm instanceof SVGElement) {
					arm.style.setProperty(
						"--axis",
						String(Math.min(1, Math.max(0, Math.abs(value)))),
					);
				}
			}
		};

		apply();
		const subscription = signals.workspace.subscribe(apply);

		return () => subscription.unsubscribe();
	}, [focusSymbol]);

	return (
		<div ref={root} className="flex h-full flex-col">
			<Panel size="lg">
				<Flex className="mb-2 font-semibold text-(--f1) text-xs">
					Regime radar
				</Flex>
				<Flex className="mb-2 font-mono text-[9.5px] text-(--f4)">
					{focusSymbol} · SNM axes
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
							data-axis={axis.label}
							style={{
								transform: "scale(clamp(0, var(--axis, 0), 1))",
								transformOrigin: "110px 105px",
							}}
						>
							<line
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
	);
};
