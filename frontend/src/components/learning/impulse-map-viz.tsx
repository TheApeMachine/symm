import * as d3 from "d3";
import { Grid2X2, Pause, Play, RefreshCw, Waves } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import type { ImpulseNode } from "./types";

interface ImpulseMapVizProps {
	data: ImpulseNode[];
	regions?: Array<{
		id: number;
		strength: number;
		authority: number;
		members: number;
	}>;
	activeEvents?: Array<{
		label: string;
		activity: number;
	}>;
	className?: string;
}

const REGION_PALETTE = [
	{ name: "Region A", fill: "rgba(74, 169, 197, 0.14)", stroke: "#4aa9c5", text: "#7fbacb", glow: "rgba(74, 169, 197, 0.4)" },
	{ name: "Region B", fill: "rgba(232, 163, 61, 0.14)", stroke: "#e8a33d", text: "#f0b865", glow: "rgba(232, 163, 61, 0.4)" },
	{ name: "Region C", fill: "rgba(140, 198, 101, 0.14)", stroke: "#8cc665", text: "#a5d684", glow: "rgba(140, 198, 101, 0.4)" },
	{ name: "Region D", fill: "rgba(180, 130, 220, 0.14)", stroke: "#b482dc", text: "#c99eeb", glow: "rgba(180, 130, 220, 0.4)" },
	{ name: "Region E", fill: "rgba(240, 84, 79, 0.14)", stroke: "#f0544f", text: "#f67e7a", glow: "rgba(240, 84, 79, 0.4)" },
	{ name: "Region F", fill: "rgba(90, 210, 180, 0.14)", stroke: "#5ad2b4", text: "#7ee0c8", glow: "rgba(90, 210, 180, 0.4)" },
	{ name: "Region G", fill: "rgba(247, 183, 49, 0.14)", stroke: "#f7b731", text: "#fad375", glow: "rgba(247, 183, 49, 0.4)" },
	{ name: "Region H", fill: "rgba(95, 39, 205, 0.14)", stroke: "#5f27cd", text: "#8854d0", glow: "rgba(95, 39, 205, 0.4)" },
];

export const ImpulseMapViz: React.FC<ImpulseMapVizProps> = ({
	data,
	regions = [],
	activeEvents = [],
	className,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const svgRef = useRef<SVGSVGElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [isPlaying, setIsPlaying] = useState(true);
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");
	const [hoveredNode, setHoveredNode] = useState<{
		node: ImpulseNode;
		x: number;
		y: number;
	} | null>(null);

	// Resize Observer
	useEffect(() => {
		const observeTarget = containerRef.current;
		if (!observeTarget || typeof ResizeObserver === "undefined") return;

		const resizeObserver = new ResizeObserver((entries) => {
			if (!entries[0]) return;
			const { width, height } = entries[0].contentRect;
			if (width > 0 && height > 0) {
				setDimensions({ width, height });
			}
		});

		resizeObserver.observe(observeTarget);
		return () => resizeObserver.unobserve(observeTarget);
	}, []);

	// Group nodes into distinct regions by cluster
	const regionClusters = useMemo(() => {
		const map = new Map<number, ImpulseNode[]>();
		for (const d of data) {
			const c = d.cluster ?? 0;
			let list = map.get(c);
			if (!list) {
				list = [];
				map.set(c, list);
			}
			list.push(d);
		}
		return Array.from(map.entries()).map(([clusterId, nodes], idx) => ({
			clusterId,
			nodes,
			palette: REGION_PALETTE[idx % REGION_PALETTE.length],
			label: `Region ${String.fromCharCode(65 + idx)}`,
		}));
	}, [data]);

	// Render D3 visualization
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0 || dimensions.height === 0)
			return;

		const svg = d3.select(svgRef.current);
		svg.selectAll("*").remove();

		const width = dimensions.width;
		const height = dimensions.height;
		const padding = 50;
		const innerW = width - padding * 2;
		const innerH = height - padding * 2;

		const g = svg.append("g").attr("class", "viewport");

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.5, 4])
			.on("zoom", (e) => {
				g.attr("transform", e.transform);
			});
		svg.call(zoom);

		// Compute screen coordinates for each node
		const positionedNodes = data.map((d) => {
			const px = typeof d.x === "number" ? Math.max(0, Math.min(1, d.x)) : 0.5;
			const py = typeof d.y === "number" ? Math.max(0, Math.min(1, d.y)) : 0.5;
			return {
				...d,
				screenX: padding + px * innerW,
				screenY: padding + py * innerH,
			};
		});

		// Region Hulls and Boundaries Layer (Watershed)
		const regionLayer = g.append("g").attr("class", "regions");

		if (layoutMode === "regions" && regionClusters.length > 0) {
			for (const cluster of regionClusters) {
				const clusterNodes = positionedNodes.filter(
					(n) => (n.cluster ?? 0) === cluster.clusterId,
				);
				if (clusterNodes.length < 3) continue;

				const points: [number, number][] = clusterNodes.map((n) => [
					n.screenX,
					n.screenY,
				]);
				const hull = d3.polygonHull(points);

				if (hull && hull.length >= 3) {
					// Draw smooth expanded region hull
					const centroid = d3.polygonCentroid(hull);
					const expandedHull = hull.map(([hx, hy]) => {
						const dx = hx - centroid[0];
						const dy = hy - centroid[1];
						const dist = Math.hypot(dx, dy);
						const scale = dist > 0 ? (dist + 16) / dist : 1;
						return [centroid[0] + dx * scale, centroid[1] + dy * scale] as [
							number,
							number,
						];
					});

					const hullPath = d3.line().curve(d3.curveCardinalClosed.tension(0.4))(
						expandedHull,
					);

					if (hullPath) {
						regionLayer
							.append("path")
							.attr("d", hullPath)
							.attr("fill", cluster.palette.fill)
							.attr("stroke", cluster.palette.stroke)
							.attr("stroke-width", 1.5)
							.attr("stroke-dasharray", "4,3")
							.attr("stroke-opacity", 0.6)
							.style("pointer-events", "none");
					}

					// Region Badge at Centroid
					const badgeGroup = regionLayer
						.append("g")
						.attr(
							"transform",
							`translate(${centroid[0]}, ${centroid[1]})`,
						)
						.style("pointer-events", "none");

					badgeGroup
						.append("rect")
						.attr("x", -44)
						.attr("y", -11)
						.attr("width", 88)
						.attr("height", 22)
						.attr("rx", 3)
						.attr("fill", "rgba(18, 22, 28, 0.88)")
						.attr("stroke", cluster.palette.stroke)
						.attr("stroke-width", 1)
						.attr("opacity", 0.9);

					badgeGroup
						.append("text")
						.attr("x", 0)
						.attr("y", -1)
						.attr("text-anchor", "middle")
						.attr("fill", cluster.palette.text)
						.attr("font-size", "9px")
						.attr("font-weight", "bold")
						.attr("font-family", "monospace")
						.text(cluster.label);

					badgeGroup
						.append("text")
						.attr("x", 0)
						.attr("y", 8)
						.attr("text-anchor", "middle")
						.attr("fill", "var(--f4)")
						.attr("font-size", "7.5px")
						.attr("font-family", "monospace")
						.text(`${cluster.nodes.length} cells`);
				}
			}
		}

		// Node Points Layer
		const nodeLayer = g.append("g").attr("class", "nodes");

		const getColor = (activation: number, clusterIdx: number) => {
			if (activation <= 0.02) {
				return layoutMode === "regions"
					? REGION_PALETTE[clusterIdx % REGION_PALETTE.length].stroke
					: "rgba(105, 177, 203, 0.45)";
			}
			if (activation < 0.25) return "#4aa9c5";
			if (activation < 0.5) return "#73be68";
			if (activation < 0.75) return "#e8a33d";
			return "#f0544f";
		};

		nodeLayer
			.selectAll<SVGCircleElement, (typeof positionedNodes)[0]>("circle")
			.data(positionedNodes, (d) => d.id)
			.enter()
			.append("circle")
			.attr("cx", (d) => d.screenX)
			.attr("cy", (d) => d.screenY)
			.attr("r", (d) => (d.activation > 0.02 ? 3.5 + Math.min(d.activation * 6, 6) : 3))
			.attr("fill", (d) => (d.activation > 0.02 ? getColor(d.activation, d.cluster ?? 0) : "rgba(22, 28, 36, 0.9)"))
			.attr("stroke", (d) => getColor(d.activation, d.cluster ?? 0))
			.attr("stroke-width", (d) => (d.activation > 0.02 ? 1.5 : 1))
			.style("cursor", "pointer")
			.on("mouseover", function (e: MouseEvent, d) {
				d3.select(this)
					.attr("stroke", "var(--acc)")
					.attr("stroke-width", 2.5)
					.attr("r", 7);
				const rect = containerRef.current?.getBoundingClientRect();
				const x = rect ? e.clientX - rect.left : e.clientX;
				const y = rect ? e.clientY - rect.top : e.clientY;
				setHoveredNode({ node: d, x, y });
			})
			.on("mouseout", function (_, d) {
				d3.select(this)
					.attr("stroke", getColor(d.activation, d.cluster ?? 0))
					.attr("stroke-width", d.activation > 0.02 ? 1.5 : 1)
					.attr("r", d.activation > 0.02 ? 3.5 + Math.min(d.activation * 6, 6) : 3);
				setHoveredNode(null);
			});
	}, [dimensions, data, layoutMode, regionClusters]);

	return (
		<div
			className={cn(
				"flex flex-col w-full h-full bg-(--bg) text-(--f2) font-mono",
				className,
			)}
		>
			{/* Top Bar Controls */}
			<div className="h-8 border-(--line) border-b flex items-center justify-between px-3 text-xs bg-(--surface) shrink-0">
				<div className="flex gap-4 items-center">
					<span className="text-(--acc) border-(--acc) border-b py-1 text-[11px] font-bold tracking-wider">
						IMPULSE MAP
					</span>
					<span className="text-(--f3) text-[11px]">
						{data.length} numeric cells · {regions.length > 0 ? regions.length : regionClusters.length} regions
					</span>
				</div>

				<div className="flex items-center gap-3">
					<div className="flex items-center gap-1 bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px]">
						<button
							type="button"
							onClick={() => setLayoutMode("grid")}
							className={cn(
								"px-2.5 py-0.5 rounded transition-colors flex items-center gap-1",
								layoutMode === "grid"
									? "bg-(--raised) text-(--acc) font-bold"
									: "text-(--f3) hover:text-(--f1)",
							)}
						>
							<Grid2X2 className="w-3 h-3" />
							Grid
						</button>
						<button
							type="button"
							onClick={() => setLayoutMode("regions")}
							className={cn(
								"px-2.5 py-0.5 rounded transition-colors flex items-center gap-1",
								layoutMode === "regions"
									? "bg-(--raised) text-(--acc) font-bold"
									: "text-(--f3) hover:text-(--f1)",
							)}
						>
							<Waves className="w-3 h-3" />
							Watershed
						</button>
					</div>

					<div className="flex items-center gap-1.5 text-(--f3) text-[10px]">
						<div className="w-2 h-2 rounded-full bg-(--acc) opacity-80" />
						<span>Top N region split</span>
					</div>

					<div className="h-3 w-px bg-(--line)" />

					<button
						type="button"
						onClick={() => setHoveredNode(null)}
						className="text-(--f3) hover:text-(--f1) transition-colors p-1"
						title="Reset map view"
					>
						<RefreshCw className="w-3 h-3" />
					</button>
					<button
						type="button"
						onClick={() => setIsPlaying(!isPlaying)}
						className="text-(--acc) hover:text-(--f1) transition-colors p-1"
						title={isPlaying ? "Pause stream" : "Resume stream"}
					>
						{isPlaying ? (
							<Pause className="w-3 h-3" />
						) : (
							<Play className="w-3 h-3" />
						)}
					</button>
				</div>
			</div>

			{/* Main Canvas Area */}
			<div
				ref={containerRef}
				className="flex-1 relative bg-(--sunken) overflow-hidden"
			>
				{/* Background Coordinate Grid */}
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.15]"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "32px 32px",
					}}
				/>

				{data.length === 0 && (
					<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-xs tracking-wider">
						Waiting for numeric cells from streaming tape...
					</div>
				)}

				<svg ref={svgRef} className="w-full h-full absolute inset-0" />

				{/* Hover Tooltip */}
				{hoveredNode && (
					<div
						className="absolute z-30 pointer-events-none bg-(--surface) border-(--line2) border rounded p-2 text-[10px] text-(--f1) shadow-xl flex flex-col gap-1 min-w-36"
						style={{
							left: Math.min(hoveredNode.x + 12, dimensions.width - 160),
							top: Math.min(hoveredNode.y + 12, dimensions.height - 110),
						}}
					>
						<div className="flex items-center justify-between border-(--line) border-b pb-1 font-bold">
							<span className="text-(--acc)">{hoveredNode.node.label}</span>
							<span className="text-(--f4)">#{hoveredNode.node.id}</span>
						</div>
						<div className="flex justify-between text-(--f3)">
							<span>Value:</span>
							<span className="text-(--f1)">
								{hoveredNode.node.value !== undefined
									? hoveredNode.node.value.toFixed(4)
									: "—"}
							</span>
						</div>
						<div className="flex justify-between text-(--f3)">
							<span>Activity:</span>
							<span className="text-(--up)">
								{((hoveredNode.node.activation || 0) * 100).toFixed(1)}%
							</span>
						</div>
						<div className="flex justify-between text-(--f3)">
							<span>SNR:</span>
							<span className="text-(--info)">
								{(hoveredNode.node.snr || 0).toFixed(2)}
							</span>
						</div>
					</div>
				)}

				{/* Active Precursor Region Sequence Overlay */}
				{activeEvents.length > 0 && (
					<div className="absolute top-3 left-3 w-64 bg-(--surface)/95 backdrop-blur border-(--line) border rounded p-2.5 pointer-events-none shadow-xl text-[10px]">
						<div className="uppercase tracking-widest text-(--f4) mb-1.5 flex items-center justify-between font-bold">
							<span>Precursor Region Sequence</span>
							{isPlaying && (
								<span className="w-1.5 h-1.5 rounded-full bg-(--up) animate-pulse" />
							)}
						</div>
						<div className="space-y-1 font-mono">
							{activeEvents.slice(0, 4).map((evt) => (
								<div
									key={evt.label}
									className="flex justify-between items-center text-(--f2)"
								>
									<span className="truncate">{evt.label}</span>
									<span className="text-(--acc) shrink-0 ml-2 font-bold">
										{(evt.activity * 100).toFixed(0)}%
									</span>
								</div>
							))}
						</div>
						<div className="mt-2 pt-1.5 border-t border-(--line) text-[9px] text-(--f4) truncate">
							Sequence: [{activeEvents.slice(0, 4).map((e) => e.label.split(" ")[0]).join(" → ")}]
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
