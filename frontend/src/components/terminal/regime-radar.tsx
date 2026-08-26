import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";
import { measurementsStore, useSubscribe } from "#/providers/ws-stores";

const radarAxes = [
	{ label: "volatility", source: "hawkes", metric: "spectral_radius", x: 0, y: -1 },
	{ label: "trend", source: "pumpdump", metric: "trend", x: 0.951, y: -0.309 },
	{ label: "drive", source: "cvd", metric: "drive", x: 0.588, y: 0.809 },
	{ label: "starved", source: "cvd", metric: "starvation", x: -0.588, y: 0.809 },
	{ label: "chop", source: "cvd", metric: "balance", x: -0.951, y: -0.309 },
];

export const RadarPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(measurementsStore, (state) => {
		for (const axis of radarAxes) {
			const row = state.measurements[`${axis.source}\u0000${focusSymbol}`]?.latest();
			const value = row?.metrics?.[axis.metric]?.normalized ?? 0;
			const arm = root.current?.querySelector<SVGElement>(`[data-axis="${axis.label}"]`);

			if (arm instanceof SVGElement) {
				arm.style.setProperty("--axis", String(Math.min(1, Math.max(0, value))));
			}
		}
	}, [focusSymbol]);

	return (
		<div ref={root} className="flex h-full flex-col">
			<Panel size="lg">
				<Flex className="mb-2 font-semibold text-(--f1) text-xs">Regime radar</Flex>
				<Flex className="mb-2 font-mono text-[9.5px] text-(--f4)">{focusSymbol} · normalized axes</Flex>
				<svg viewBox="0 0 220 210" className="block w-full">
					<title>Regime radar</title>
					<polygon points="110,21 190,79 159,173 61,173 30,79" fill="none" stroke="#3a342b" />
					<polygon points="110,49 163,87 142,154 78,154 57,87" fill="none" stroke="#2b251e" />
					<polygon points="110,77 137,94 126,134 94,134 83,94" fill="none" stroke="#2b251e" />
					{radarAxes.map((axis) => (
						<line key={`spoke:${axis.label}`} x1="110" y1="105" x2={110 + axis.x * 84} y2={105 + axis.y * 84} stroke="#2b251e" />
					))}
					{radarAxes.map((axis) => (
						<g
							key={`arm:${axis.label}`}
							data-axis={axis.label}
							style={{ transform: "scale(clamp(0, var(--axis, 0), 1))", transformOrigin: "110px 105px" }}
						>
							<line x1="110" y1="105" x2={110 + axis.x * 84} y2={105 + axis.y * 84} stroke="#e8a33d" strokeWidth="1.6" />
							<circle cx={110 + axis.x * 84} cy={105 + axis.y * 84} r="2.6" fill="#e8a33d" />
						</g>
					))}
					{radarAxes.map((axis) => (
						<text key={`label:${axis.label}`} x={110 + axis.x * 98} y={105 + axis.y * 98} textAnchor="middle" fontSize="9" fill="#938a7e">
							{axis.label}
						</text>
					))}
				</svg>
			</Panel>
		</div>
	);
};
