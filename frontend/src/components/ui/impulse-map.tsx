import {
	type ComponentProps,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";

export interface ImpulsePoint {
	id: number;
	source: string;
	label: string;
	x: number;
	y: number;
	snr: number;
	activation: number;
	energy: number;
	authority: number;
	present: boolean;
}

export interface ImpulseRegion {
	id: number;
	source: string;
	snr?: number;
	authority?: number;
	members?: number;
}

export interface ImpulseContour {
	id: string;
	label: string;
	points: { x: number; y: number }[];
}

export interface ImpulseConnection {
	from: number;
	to: number;
	strength?: number;
}

export type ImpulseMapProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	points?: ImpulsePoint[];
	/** The hot regions: those above the hot-region split (Otsu split). */
	regions?: ImpulseRegion[];
	contours?: ImpulseContour[];
	connections?: ImpulseConnection[];
	title?: string;
	/** Ingress bootstrap lifecycle phase: WARMING_SIGNALS, FORMING_MAP, SETTLED */
	phase?: string;
	/**
	 * Drawing extent in producer coordinate units, centered at the origin.
	 * Without one the drawing fits the supplied points.
	 */
	viewport?: { width: number; height: number };
	fallback?: boolean;
};


/* Heat from the panel line through blue and green to the accent. */
const HEAT = ["#3a342b", "#7fbacb", "#9cc06e", "#e8a33d"];

const mix = (from: string, to: string, t: number) => {
	const channel = (hex: string, offset: number) =>
		Number.parseInt(hex.slice(offset, offset + 2), 16);
	const blend = (offset: number) =>
		Math.round(
			channel(from, offset) + (channel(to, offset) - channel(from, offset)) * t,
		);
	return `rgb(${blend(1)}, ${blend(3)}, ${blend(5)})`;
};

/* heat maps a share in [0,1] onto the heat scale. */
export const heat = (share: number) => {
	const clamped = Math.min(1, Math.max(0, share));
	const scaled = clamped * (HEAT.length - 1);
	const index = Math.min(HEAT.length - 2, Math.floor(scaled));
	return mix(HEAT[index], HEAT[index + 1], scaled - index);
};

/*
Draws supplied positions and relationships without simulating signals or
deciding which evidence is hot: positions, activation and the hot regions all
come from the producer. The view owns only the layout toggle (the original
lattice or the arrangement sympathy produced) and hover inspection.
*/
export const ImpulseMap = ({
	points: suppliedPoints,
	regions: suppliedRegions,
	contours,
	connections: suppliedConnections,
	title = "Map",
	phase,
	viewport,
	fallback = false,
	className,
	...props
}: ImpulseMapProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const [size, setSize] = useState({ width: 640, height: 400 });
	const [layout, setLayout] = useState<"grid" | "regions">("regions");
	const [selected, setSelected] = useState<number | null>(null);

	const points = suppliedPoints ?? [];
	const regions = suppliedRegions ?? [];
	const connections = suppliedConnections ?? [];

	useEffect(() => {
		const target = ref.current;
		if (!target || typeof ResizeObserver === "undefined") return;
		const observer = new ResizeObserver((entries) => {
			const box = entries[0]?.contentRect;
			if (box && box.width > 0 && box.height > 0)
				setSize({ width: box.width, height: box.height });
		});
		observer.observe(target);
		return () => observer.disconnect();
	}, []);

	const placed = useMemo(
		() => place(points ?? [], layout, size, viewport),
		[points, layout, size, viewport],
	);
	const byId = new Map(placed.points.map((point) => [point.id, point]));
	const hot = new Set((regions ?? []).map((region) => region.source));
	const strongest = Math.max(
		0,
		...(points ?? []).map((point) => Math.abs(point.activation)),
	);
	const inspected = selected === null ? undefined : byId.get(selected);
	const members = new Map<string, number>();
	for (const point of points ?? []) {
		if (hot.has(point.source))
			members.set(point.source, (members.get(point.source) ?? 0) + 1);
	}

	return (
		<Flex.Column
			className={cn(
				"min-h-64 overflow-hidden rounded border border-(--line) bg-(--surface) font-mono text-xs text-(--f3)",
				className,
			)}
			{...props}
		>
			<Flex.Row className="h-8 shrink-0 items-center justify-between border-(--line) border-b bg-(--sunken) px-4">
				<span className="border-(--acc) border-b py-1.5 text-(--acc)">
					{title}
				</span>
				<Flex.Row className="items-center gap-3">
					{phase && (
						<span
							className={cn(
								"rounded px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider",
								phase === "SETTLED"
									? "bg-(--up)/15 text-(--up) border border-(--up)/30"
									: phase === "FORMING_MAP"
										? "bg-(--acc)/15 text-(--acc) border border-(--acc)/30"
										: "bg-(--warning)/15 text-(--warning) border border-(--warning)/30",
							)}
						>
							{phase === "SETTLED"
								? "SETTLED"
								: phase === "FORMING_MAP"
									? "FORMING · TOKENS DISABLED"
									: "WARMING SENSORIUM"}
						</span>
					)}
					<Flex.Row className="items-center gap-1 rounded border border-(--line) bg-(--raised) p-0.5">
						{(
							[
								["grid", "Initial grid"],
								["regions", "Sympathy clustering"],
							] as const
						).map(([mode, label]) => (
							<button
								key={mode}
								type="button"
								onClick={() => setLayout(mode)}
								className={cn(
									"rounded px-2.5 py-0.5 transition-colors",
									layout === mode
										? "bg-(--line) text-(--acc)"
										: "text-(--f4) hover:text-(--f2)",
								)}
							>
								{label}
							</button>
						))}
					</Flex.Row>
					<Flex.Row className="items-center gap-1 text-(--f4)">
						<span className="inline-block size-2 rounded-sm bg-(--acc) opacity-80" />
						<span>Hot-region split · Otsu</span>
					</Flex.Row>
				</Flex.Row>
			</Flex.Row>

			<div ref={ref} className="relative min-h-0 flex-1 bg-(--sunken)">
				<div
					className="pointer-events-none absolute inset-0 opacity-[0.15]"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "40px 40px",
					}}
				/>
				{!points?.length && (
					<span className="absolute inset-0 flex items-center justify-center tracking-widest text-(--f4)">
						No impulse data
					</span>
				)}
				<svg
					role="img"
					aria-label="Impulse map"
					width={size.width}
					height={size.height}
					className="absolute inset-0"
				>
					<title>Impulse map</title>
					{layout === "regions" &&
						contours?.map((contour) => (
							<polygon
								key={contour.id}
								points={contour.points
									.map((point) => {
										const at = placed.project(point.x, point.y);
										return `${at.x},${at.y}`;
									})
									.join(" ")}
								className="fill-(--acc)/10 stroke-(--acc)/40"
							>
								<title>{contour.label}</title>
							</polygon>
						))}
					{layout === "regions" &&
						connections?.map((connection) => {
							const from = byId.get(connection.from);
							const to = byId.get(connection.to);

							if (!from || !to)
								throw new Error(
									`ImpulseMap: connection ${connection.from} -> ${connection.to} references a missing point`,
								);

							const strength = connection.strength ?? 0;
							const isAttraction = strength >= 0;
							const magnitude = Math.min(1, Math.max(0.15, Math.abs(strength)));
							const strokeColor = isAttraction ? "var(--acc)" : "var(--down)";
							const strokeWidth = 0.5 + 1.5 * magnitude;
							const strokeOpacity = 0.2 + 0.6 * magnitude;

							return (
								<line
									key={`${connection.from}:${connection.to}`}
									x1={from.px}
									y1={from.py}
									x2={to.px}
									y2={to.py}
									stroke={strokeColor}
									strokeOpacity={strokeOpacity}
									strokeWidth={strokeWidth}
								>
									<title>{`sympathy: ${connection.from} ↔ ${connection.to} · strength: ${strength.toFixed(3)} (${isAttraction ? "attraction" : "repulsion"})`}</title>
								</line>
							);
						})}
					{placed.points.map((point) => {
						const lit = hot.has(point.source);
						// A square-root share keeps quieter movement visible beside
						// the strongest instead of fading it into the panel.
						const share =
							strongest > 0
								? Math.sqrt(Math.abs(point.activation) / strongest)
								: 0;
						return (
							// biome-ignore lint/a11y/useSemanticElements: SVG has no native button; this circle supports click, Enter, Space and focus inspection.
							<circle
								role="button"
								key={point.id}
								cx={point.px}
								cy={point.py}
								r={placed.radius * (0.45 + 0.55 * point.snr)}
								fill={point.present ? heat(share) : "var(--raised)"}
								stroke={lit ? "var(--acc)" : "var(--sunken)"}
								strokeWidth={lit ? 1.5 : 1}
								tabIndex={0}
								aria-label={point.label}
								onMouseEnter={() => setSelected(point.id)}
								onMouseLeave={() => setSelected(null)}
								onClick={() => setSelected(point.id)}
								onKeyDown={(event) => {
									if (event.key === "Enter" || event.key === " ") {
										event.preventDefault();
										setSelected(point.id);
									}
								}}
								onFocus={() => setSelected(point.id)}
								onBlur={() => setSelected(null)}
							>
								<title>{`coordinate ${point.id} · region ${point.source} · SNR ${point.snr.toFixed(2)}`}</title>
							</circle>
						);
					})}
				</svg>

				{!!regions?.length && (
					<div className="pointer-events-none absolute top-4 left-4 w-56 rounded border border-(--line) bg-(--surface)/85 p-3 shadow-xl backdrop-blur">
						<div className="mb-2 text-[10px] uppercase tracking-widest text-(--f4)">
							Hot regions
						</div>
						<div className="space-y-1.5">
							{regions.map((region) => (
								<div
									key={region.id}
									className="flex items-center justify-between text-(--f2)"
								>
									<span className="truncate">region {region.source}</span>
									<span className="text-(--acc)">
										{members.get(region.source) ?? 0} coordinates
									</span>
								</div>
							))}
						</div>
					</div>
				)}

				{inspected && (
					<div
						className="absolute right-4 bottom-4 rounded border border-(--line) bg-(--surface)/90 px-3 py-2 text-(--f2)"
						aria-live="polite"
					>
						coordinate {inspected.id} · region {inspected.source} · SNR{" "}
						{inspected.snr.toFixed(2)} · authority{" "}
						{inspected.authority.toFixed(3)}
					</div>
				)}
			</div>
		</Flex.Column>
	);
};

/*
place projects the producer's coordinates onto the drawing. In grid mode each
point sits on the lattice its coordinate started on; in regions mode it sits
where the arrangement moved it. Either way the extent is fitted to the drawing.
*/
const place = (
	points: ImpulsePoint[],
	layout: "grid" | "regions",
	size: { width: number; height: number },
	viewport?: { width: number; height: number },
) => {
	const columns = Math.max(1, Math.ceil(Math.sqrt(points.length)));
	const source = points.map((point) =>
		layout === "grid"
			? { x: point.id % columns, y: Math.floor(point.id / columns) }
			: { x: point.x, y: point.y },
	);

	let left = viewport
		? -viewport.width / 2
		: Math.min(...source.map((p) => p.x));
	let right = viewport
		? viewport.width / 2
		: Math.max(...source.map((p) => p.x));
	let top = viewport
		? -viewport.height / 2
		: Math.min(...source.map((p) => p.y));
	let bottom = viewport
		? viewport.height / 2
		: Math.max(...source.map((p) => p.y));

	if (!points.length) {
		left = -1;
		right = 1;
		top = -1;
		bottom = 1;
	}

	const spanX = right - left || 1;
	const spanY = bottom - top || 1;
	const margin = 32;
	const scale = Math.min(
		(size.width - 2 * margin) / spanX,
		(size.height - 2 * margin) / spanY,
	);
	const offsetX = (size.width - spanX * scale) / 2;
	const offsetY = (size.height - spanY * scale) / 2;
	const project = (x: number, y: number) => ({
		x: offsetX + (x - left) * scale,
		y: offsetY + (y - top) * scale,
	});
	const spacing = Math.max(
		4,
		Math.min(size.width, size.height) / Math.max(1, columns + 1),
	);

	return {
		project,
		radius: spacing * 0.4,
		points: points.map((point, index) => {
			const at = project(source[index].x, source[index].y);
			return { ...point, px: at.x, py: at.y };
		}),
	};
};
