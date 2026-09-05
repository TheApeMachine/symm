import { Canvas } from "#/components/ui/canvas";
import type { Point, Region } from "./state";

export const ImpulseMap = ({ points, regions }: { points: Point[]; regions: Region[] }) => {
	const extent = Math.max(...points.flatMap((point) => [Math.abs(point.x), Math.abs(point.y)]), 0);
	const scale = extent > 0 ? 240 / extent : 0;
	const energy = Math.max(...points.map((point) => point.energy), 0);
	const peaks = new Set(regions.map((region) => region.id));
	return (
		<Canvas title="Impulse map" meta={`${points.length} numeric cells · ${regions.length} hot regions`} className="min-h-80 flex-1" footer="Position: learned affinity · brightness: current activity · hover to inspect">
			<svg viewBox="-300 -300 600 600" className="h-full w-full" role="img" aria-label="Live self-organizing numeric impulse map">
				<path d="M-270 0H270 M0-270V270" stroke="var(--line)" strokeWidth="0.5" />
				{points.map((point) => {
					const light = energy > 0 ? Math.sqrt(point.energy / energy) : 0;
					return <circle key={point.id} cx={point.x * scale} cy={point.y * scale} r={peaks.has(point.id) ? 6 : 3}
						fill={peaks.has(point.id) ? "var(--acc)" : "var(--info)"} opacity={point.present ? 0.15 + 0.85 * light : 0.08}>
						<title>{`#${point.id} ${point.source} / ${point.label}\nValue ${point.value}\nActivity ${point.energy}\nAuthority ${point.authority}\n${point.present ? "Present" : "Absent on latest update"}`}</title>
					</circle>;
				})}
			</svg>
		</Canvas>
	);
};
