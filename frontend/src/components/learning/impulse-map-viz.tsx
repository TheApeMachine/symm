import * as d3 from "d3";
import { Grid2X2, Pause, Play, RefreshCw, Waves } from "lucide-react";
import { useEffect, useRef, useState } from "react";
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

	const simulationRef = useRef<d3.Simulation<ImpulseNode, undefined> | null>(
		null,
	);
	const nodesRef = useRef<ImpulseNode[]>([]);

	// Keep internal mutable nodes in sync with real incoming data
	useEffect(() => {
		if (data.length === 0) {
			nodesRef.current = [];
			return;
		}

		// Update or merge nodes
		const existingMap = new Map<string, ImpulseNode>();
		for (const n of nodesRef.current) {
			existingMap.set(n.id, n);
		}

		const merged: ImpulseNode[] = data.map((d) => {
			const old = existingMap.get(d.id);
			if (!old) {
				return { ...d };
			}
			return {
				...d,
				x: old.x ?? d.x,
				y: old.y ?? d.y,
				vx: old.vx,
				vy: old.vy,
			};
		});

		nodesRef.current = merged;

		// Gently heat simulation if running
		if (simulationRef.current && isPlaying) {
			simulationRef.current.nodes(nodesRef.current);
			simulationRef.current.alpha(0.05).restart();
		}
	}, [data, isPlaying]);

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

	// Initialize D3 Visualization
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0 || dimensions.height === 0)
			return;

		const svg = d3.select(svgRef.current);
		svg.selectAll("*").remove();

		const width = dimensions.width;
		const height = dimensions.height;

		const g = svg.append("g").attr("class", "viewport");

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.4, 4])
			.on("zoom", (e) => {
				g.attr("transform", e.transform);
			});
		svg.call(zoom);

		const contourLayer = g.append("g").attr("class", "contours");
		const linkLayer = g.append("g").attr("class", "links");
		const nodeLayer = g.append("g").attr("class", "nodes");

		const nodes = nodesRef.current;
		const count = Math.max(nodes.length, 1);
		const cols = Math.ceil(Math.sqrt(count * (width / height)));
		const rows = Math.ceil(count / cols);
		const stepX = (width * 0.8) / Math.max(1, cols - 1);
		const stepY = (height * 0.8) / Math.max(1, rows - 1);
		const startX = width * 0.1;
		const startY = height * 0.1;

		nodes.forEach((n, i) => {
			const hasPos =
				typeof n.x === "number" &&
				typeof n.y === "number" &&
				!Number.isNaN(n.x) &&
				!Number.isNaN(n.y) &&
				(n.x > 0 || n.y > 0) &&
				n.x <= 1 &&
				n.y <= 1;

			const posX = typeof n.x === "number" ? n.x : 0;
			const posY = typeof n.y === "number" ? n.y : 0;

			n.gridX = hasPos
				? startX + posX * (width * 0.8)
				: startX + (i % cols) * stepX;
			n.gridY = hasPos
				? startY + posY * (height * 0.8)
				: startY + Math.floor(i / cols) * stepY;

			if (n.x === undefined || Number.isNaN(n.x) || n.x <= 1) {
				n.x = n.gridX;
				n.y = n.gridY;
			}
		});

		const colorScale = d3
			.scaleSequential((t) =>
				d3.interpolateRgbBasis(["#1f1a14", "#7fbacb", "#9cc06e", "#e8a33d"])(t),
			)
			.domain([0, 1]);

		const simulation = d3
			.forceSimulation(nodes)
			.force(
				"x",
				d3.forceX<ImpulseNode>((d) => d.gridX ?? width / 2).strength(0.3),
			)
			.force(
				"y",
				d3.forceY<ImpulseNode>((d) => d.gridY ?? height / 2).strength(0.3),
			)
			.force(
				"collide",
				d3
					.forceCollide<ImpulseNode>()
					.radius((d) => (d.snr ?? 0) * 1.5 + 2)
					.iterations(1)
					.strength(0.1),
			)
			.force("charge", d3.forceManyBody().strength(-2))
			.force("center", d3.forceCenter(width / 2, height / 2).strength(0.01))
			.alphaDecay(0.02);

		simulationRef.current = simulation;

		const nodeElements = nodeLayer
			.selectAll<SVGCircleElement, ImpulseNode>("circle")
			.data(nodes, (d) => d.id)
			.enter()
			.append("circle")
			.attr("r", (d) => Math.max(2, (d.snr ?? 0) * 1.5 + 2))
			.attr("fill", (d) => colorScale(d.activation || 0))
			.attr("stroke", "var(--line)")
			.attr("stroke-width", 1)
			.style("cursor", "pointer")
			.on("mouseover", function (e: MouseEvent, d: ImpulseNode) {
				d3.select(this).attr("stroke", "var(--acc)").attr("stroke-width", 2);
				const rect = containerRef.current?.getBoundingClientRect();
				const x = rect ? e.clientX - rect.left : e.clientX;
				const y = rect ? e.clientY - rect.top : e.clientY;
				setHoveredNode({ node: d, x, y });
			})
			.on("mouseout", function () {
				d3.select(this).attr("stroke", "var(--line)").attr("stroke-width", 1);
				setHoveredNode(null);
			});

		const computeDensity = d3
			.contourDensity<ImpulseNode>()
			.x((d) => d.x || 0)
			.y((d) => d.y || 0)
			.weight((d) => (d.activation > 0.05 ? d.activation * (d.snr ?? 0) : 0))
			.size([width, height])
			.bandwidth(25)
			.thresholds(12);

		simulation.on("tick", () => {
			if (isPlaying) {
				for (const n of nodes) {
					if (n.activation > 0) {
						n.activation = Math.max(0, n.activation - 0.002);
					}
				}
			}

			nodeElements
				.attr("cx", (d) => d.x ?? 0)
				.attr("cy", (d) => d.y ?? 0)
				.attr("fill", (d) => colorScale(d.activation || 0));

			const hasActivePrecursors = nodes.some((d) => d.activation > 0.05);

			if (nodes.length >= 3 && hasActivePrecursors) {
				const contourData = computeDensity(nodes);

				contourLayer
					.selectAll<SVGPathElement, d3.ContourMultiPolygon>("path")
					.data(contourData)
					.join("path")
					.attr("d", d3.geoPath())
					.attr("fill", (_, i) => {
						return (
							d3
								.color("#e8a33d")
								?.copy({ opacity: i * 0.015 })
								.toString() || "none"
						);
					})
					.attr("stroke", (_, i) => {
						if (i === 6) return "rgba(156, 192, 110, 0.45)";
						if (i === 10) return "rgba(232, 163, 61, 0.85)";
						return "none";
					})
					.attr("stroke-width", (_, i) => (i === 10 ? 1.5 : i === 6 ? 1 : 0));
			} else {
				contourLayer.selectAll("path").remove();
			}

			const activeLinks: { source: ImpulseNode; target: ImpulseNode }[] = [];
			const threshold = 0.4;

			for (let i = 0; i < nodes.length; i++) {
				if (nodes[i].activation < threshold) continue;
				for (let j = i + 1; j < nodes.length; j++) {
					if (nodes[j].activation < threshold) continue;
					if (nodes[i].cluster !== nodes[j].cluster) continue;

					const dx = (nodes[i].x ?? 0) - (nodes[j].x ?? 0);
					const dy = (nodes[i].y ?? 0) - (nodes[j].y ?? 0);
					const dist = Math.hypot(dx, dy);

					if (dist < 90) {
						activeLinks.push({ source: nodes[i], target: nodes[j] });
					}
				}
			}

			linkLayer
				.selectAll<
					SVGLineElement,
					{ source: ImpulseNode; target: ImpulseNode }
				>("line")
				.data(activeLinks)
				.join("line")
				.attr("x1", (d) => d.source.x ?? 0)
				.attr("y1", (d) => d.source.y ?? 0)
				.attr("x2", (d) => d.target.x ?? 0)
				.attr("y2", (d) => d.target.y ?? 0)
				.attr("stroke", "var(--acc)")
				.attr("stroke-opacity", (d) =>
					Math.min((d.source.activation + d.target.activation) / 2, 0.8),
				)
				.attr("stroke-width", 1.2);
		});

		return () => {
			simulation.stop();
		};
	}, [dimensions, isPlaying]);

	// Handle Mode Transitions (Grid vs Regions)
	useEffect(() => {
		const simulation = simulationRef.current;
		if (!simulation || dimensions.width === 0) return;

		const width = dimensions.width;
		const height = dimensions.height;

		const numClusters = Math.max(regions.length, 4);
		const angleStep = (2 * Math.PI) / numClusters;
		const radius = Math.min(width, height) * 0.28;
		const centerX = width / 2;
		const centerY = height / 2;

		const regionFoci = Array.from({ length: numClusters }, (_, idx) => ({
			x: centerX + radius * Math.cos(idx * angleStep - Math.PI / 2),
			y: centerY + radius * Math.sin(idx * angleStep - Math.PI / 2),
		}));

		if (layoutMode === "grid") {
			simulation
				.force(
					"x",
					d3.forceX<ImpulseNode>((d) => d.gridX ?? width / 2).strength(0.2),
				)
				.force(
					"y",
					d3.forceY<ImpulseNode>((d) => d.gridY ?? height / 2).strength(0.2),
				)
				.force("collide", null)
				.force("charge", null)
				.alphaDecay(0.015);

			d3.select(svgRef.current)
				.select(".contours")
				.transition()
				.duration(600)
				.style("opacity", 0);
		} else {
			simulation
				.force(
					"x",
					d3
						.forceX<ImpulseNode>(
							(d) => regionFoci[(d.cluster ?? 0) % regionFoci.length].x,
						)
						.strength(0.08),
				)
				.force(
					"y",
					d3
						.forceY<ImpulseNode>(
							(d) => regionFoci[(d.cluster ?? 0) % regionFoci.length].y,
						)
						.strength(0.08),
				)
				.force(
					"collide",
					d3
						.forceCollide<ImpulseNode>()
						.radius((d) => (d.snr ?? 0) * 1.5 + 2)
						.iterations(2),
				)
				.force("charge", d3.forceManyBody().strength(-15))
				.alphaDecay(0.015);

			d3.select(svgRef.current)
				.select(".contours")
				.transition()
				.duration(600)
				.style("opacity", 1);
		}

		simulation.alpha(0.8).restart();
	}, [layoutMode, dimensions, regions]);

	const handleReset = () => {
		for (const n of nodesRef.current) {
			n.activation = 0;
		}
		simulationRef.current?.alpha(0.3).restart();
	};

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
						{data.length} numeric cells · {regions.length} regions
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
						<span>Otsu split threshold</span>
					</div>

					<div className="h-3 w-px bg-(--line)" />

					<button
						type="button"
						onClick={handleReset}
						className="text-(--f3) hover:text-(--f1) transition-colors p-1"
						title="Reset map"
					>
						<RefreshCw className="w-3 h-3" />
					</button>
					<button
						type="button"
						onClick={() => setIsPlaying(!isPlaying)}
						className="text-(--acc) hover:text-(--f1) transition-colors p-1"
						title={isPlaying ? "Pause simulation" : "Play simulation"}
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
				{/* Background Grid Pattern */}
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.12]"
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

				{/* Real Market Tape Updates Feed Overlay */}
				{activeEvents.length > 0 && (
					<div className="absolute top-3 left-3 w-56 bg-(--surface)/90 backdrop-blur border-(--line) border rounded p-2.5 pointer-events-none shadow-lg text-[10px]">
						<div className="uppercase tracking-widest text-(--f4) mb-1.5 flex items-center justify-between font-bold">
							<span>Active Tape Precursors</span>
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
									<span className="text-(--acc) shrink-0 ml-2">
										{(evt.activity * 100).toFixed(0)}%
									</span>
								</div>
							))}
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
