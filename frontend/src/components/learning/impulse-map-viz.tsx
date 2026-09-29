import * as d3 from "d3";
import { Grid2X2, Pause, Play, RefreshCw, Waves } from "lucide-react";
import type React from "react";
import { useEffect, useRef, useState } from "react";
import type { ImpulseNode } from "./types";

interface ImpulseMapVizProps {
	data?: ImpulseNode[];
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
	data = [],
	activeEvents = [],
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const svgRef = useRef<SVGSVGElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [isPlaying, setIsPlaying] = useState(true);
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");

	const simulationRef = useRef<d3.Simulation<ImpulseNode, undefined> | null>(
		null,
	);
	const nodesRef = useRef<ImpulseNode[]>(
		data.length > 0 ? JSON.parse(JSON.stringify(data)) : [],
	);

	// Sync incoming data
	useEffect(() => {
		if (data && data.length > 0) {
			const existingMap = new Map<string, ImpulseNode>();
			for (const node of nodesRef.current) {
				existingMap.set(node.id, node);
			}

			const merged: ImpulseNode[] = data.map((n, i) => {
				const prev = existingMap.get(n.id);
				return {
					...n,
					cluster: typeof n.cluster === "number" ? n.cluster : i % 4,
					snr: Math.max(1, n.snr || 1),
					activation: n.activation || 0,
					x: prev?.x ?? n.x,
					y: prev?.y ?? n.y,
					vx: prev?.vx,
					vy: prev?.vy,
					gridX: prev?.gridX,
					gridY: prev?.gridY,
				};
			});

			nodesRef.current = merged;
			if (simulationRef.current) {
				simulationRef.current.nodes(merged);
				simulationRef.current.alpha(0.3).restart();
			}
		}
	}, [data]);

	// Resize Observer
	useEffect(() => {
		const observeTarget = containerRef.current;
		if (!observeTarget || typeof ResizeObserver === "undefined") return;
		const resizeObserver = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setDimensions({ width, height });
				}
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

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.5, 4])
			.on("zoom", (e) => {
				g.attr("transform", e.transform);
			});
		svg.call(zoom);

		const g = svg.append("g");

		const contourLayer = g.append("g").attr("class", "contours");
		const linkLayer = g.append("g").attr("class", "links");
		const nodeLayer = g.append("g").attr("class", "nodes");

		const nodes = nodesRef.current;
		const cols = Math.max(
			1,
			Math.ceil(Math.sqrt(nodes.length * (width / height))),
		);
		const rows = Math.max(1, Math.ceil(nodes.length / cols));
		const stepX = (width * 0.8) / Math.max(1, cols - 1);
		const stepY = (height * 0.8) / Math.max(1, rows - 1);
		const startX = width * 0.1;
		const startY = height * 0.1;

		for (let i = 0; i < nodes.length; i++) {
			const n = nodes[i];
			n.gridX = startX + (i % cols) * stepX;
			n.gridY = startY + Math.floor(i / cols) * stepY;

			if (n.x === undefined || Number.isNaN(n.x)) {
				n.x = n.gridX;
				n.y = n.gridY;
			}
		}

		const colorScale = d3
			.scaleSequential((t) =>
				d3.interpolateRgbBasis(["#111113", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
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
					.radius((d) => d.snr * 1.5 + 2)
					.iterations(1)
					.strength(0.1),
			)
			.force("charge", d3.forceManyBody().strength(-2))
			.force("center", d3.forceCenter(width / 2, height / 2).strength(0.01))
			.alphaDecay(0.02);

		simulationRef.current = simulation;

		const nodeElements = nodeLayer
			.selectAll("circle")
			.data(nodes)
			.enter()
			.append("circle")
			.attr("r", (d) => d.snr * 1.5 + 2)
			.attr("fill", (d) => colorScale(d.activation))
			.attr("stroke", "#050505")
			.attr("stroke-width", 1.5)
			.on("mouseover", function () {
				d3.select(this).attr("stroke", "#fbbf24").attr("stroke-width", 2);
			})
			.on("mouseout", function () {
				d3.select(this).attr("stroke", "#050505").attr("stroke-width", 1.5);
			});

		const computeDensity = d3
			.contourDensity<ImpulseNode>()
			.x((d) => d.x || 0)
			.y((d) => d.y || 0)
			.weight((d) => d.activation * d.snr)
			.size([width, height])
			.bandwidth(30)
			.thresholds(15);

		simulation.on("tick", () => {
			if (isPlaying) {
				for (const n of nodes) {
					if (n.activation > 0) {
						n.activation = Math.max(0, n.activation - 0.005);
					}
				}
			}

			nodeElements
				.attr("cx", (d) => d.x || 0)
				.attr("cy", (d) => d.y || 0)
				.attr("fill", (d) => colorScale(d.activation));

			const contourData = computeDensity(nodes);

			const paths = contourLayer.selectAll("path").data(contourData);

			paths
				.enter()
				.append("path")
				.merge(
					paths as unknown as d3.Selection<
						SVGPathElement,
						d3.ContourMultiPolygon,
						SVGGElement,
						unknown
					>,
				)
				.attr("d", d3.geoPath())
				.attr(
					"fill",
					(_, i) =>
						d3
							.color("#fbbf24")
							?.copy({ opacity: i * 0.015 })
							.toString() || "none",
				)
				.attr("stroke", (_, i) => {
					if (i === 6) return "rgba(34, 197, 94, 0.4)";
					if (i === 10) return "rgba(251, 191, 36, 0.8)";
					return "none";
				})
				.attr("stroke-width", (_, i) => (i === 10 ? 1.5 : i === 6 ? 1 : 0));

			paths.exit().remove();

			const activeLinks: { source: ImpulseNode; target: ImpulseNode }[] = [];
			const threshold = 0.6;

			for (let i = 0; i < nodes.length; i++) {
				if (nodes[i].activation < threshold) continue;
				for (let j = i + 1; j < nodes.length; j++) {
					if (nodes[j].activation < threshold) continue;
					if (nodes[i].cluster !== nodes[j].cluster) continue;

					const dx = (nodes[i].x || 0) - (nodes[j].x || 0);
					const dy = (nodes[i].y || 0) - (nodes[j].y || 0);
					const dist = Math.hypot(dx, dy);

					if (dist < 80) {
						activeLinks.push({ source: nodes[i], target: nodes[j] });
					}
				}
			}

			const links = linkLayer.selectAll("line").data(activeLinks);

			links
				.enter()
				.append("line")
				.merge(
					links as unknown as d3.Selection<
						SVGLineElement,
						{ source: ImpulseNode; target: ImpulseNode },
						SVGGElement,
						unknown
					>,
				)
				.attr("x1", (d) => d.source.x || 0)
				.attr("y1", (d) => d.source.y || 0)
				.attr("x2", (d) => d.target.x || 0)
				.attr("y2", (d) => d.target.y || 0)
				.attr("stroke", "#fbbf24")
				.attr("stroke-opacity", (d) =>
					Math.min((d.source.activation + d.target.activation) / 2, 0.8),
				)
				.attr("stroke-width", 1.5);

			links.exit().remove();
		});

		return () => {
			simulation.stop();
		};
	}, [dimensions, isPlaying]);

	// Handle Mode Transitions
	useEffect(() => {
		const simulation = simulationRef.current;
		if (!simulation || dimensions.width === 0) return;

		const width = dimensions.width;
		const height = dimensions.height;

		const foci = [
			{ x: width * 0.3, y: height * 0.3 },
			{ x: width * 0.7, y: height * 0.3 },
			{ x: width * 0.3, y: height * 0.7 },
			{ x: width * 0.7, y: height * 0.7 },
		];

		if (layoutMode === "grid") {
			simulation
				.force(
					"x",
					d3.forceX<ImpulseNode>((d) => d.gridX ?? width / 2).strength(0.15),
				)
				.force(
					"y",
					d3.forceY<ImpulseNode>((d) => d.gridY ?? height / 2).strength(0.15),
				)
				.force("collide", null)
				.force("charge", null)
				.alphaDecay(0.015);

			d3.select(svgRef.current)
				.select(".contours")
				.transition()
				.duration(1000)
				.style("opacity", 0);
		} else {
			simulation
				.force(
					"x",
					d3
						.forceX<ImpulseNode>((d) => foci[(d.cluster ?? 0) % 4].x)
						.strength(0.08),
				)
				.force(
					"y",
					d3
						.forceY<ImpulseNode>((d) => foci[(d.cluster ?? 0) % 4].y)
						.strength(0.08),
				)
				.force(
					"collide",
					d3
						.forceCollide<ImpulseNode>()
						.radius((d) => d.snr * 1.5 + 2)
						.iterations(2),
				)
				.force("charge", d3.forceManyBody().strength(-15))
				.alphaDecay(0.015);

			d3.select(svgRef.current)
				.select(".contours")
				.transition()
				.duration(1000)
				.style("opacity", 1);
		}

		simulation.alpha(1).restart();
	}, [layoutMode, dimensions]);

	return (
		<div className="flex flex-col w-full h-full">
			<div className="h-8 border-b border-[#27272a] flex items-center justify-between px-4 text-xs bg-[#09090b]">
				<div className="flex gap-4">
					<span className="text-[#fbbf24] border-b border-[#fbbf24] py-1.5">
						Map
					</span>
					<span className="text-[#52525b] py-1.5">Topography</span>
					<span className="text-[#52525b] py-1.5">Sympathy Grid</span>
				</div>
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-1 bg-[#111113] border border-[#27272a] p-1 rounded mr-2">
						<button
							type="button"
							onClick={() => setLayoutMode("grid")}
							className={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${
								layoutMode === "grid"
									? "bg-[#27272a] text-[#fbbf24]"
									: "text-[#71717a] hover:text-[#a1a1aa]"
							}`}
						>
							<Grid2X2 className="w-3.5 h-3.5" />
							Initial Grid
						</button>
						<button
							type="button"
							onClick={() => setLayoutMode("regions")}
							className={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${
								layoutMode === "regions"
									? "bg-[#27272a] text-[#fbbf24]"
									: "text-[#71717a] hover:text-[#a1a1aa]"
							}`}
						>
							<Waves className="w-3.5 h-3.5" />
							Sympathy Clustering
						</button>
					</div>

					<div className="flex items-center gap-1 text-[#52525b]">
						<div className="w-2 h-2 rounded bg-[#fbbf24] opacity-80" />
						<span>Agent Action Threshold (Otsu Split)</span>
					</div>
					<div className="h-3 w-px bg-[#27272a]" />
					<button
						type="button"
						onClick={() => {
							for (const n of nodesRef.current) n.activation = 0;
							simulationRef.current?.alpha(0.3).restart();
						}}
						className="text-[#71717a] hover:text-white transition-colors"
						title="Reset Map"
					>
						<RefreshCw className="w-3.5 h-3.5" />
					</button>
					<button
						type="button"
						onClick={() => setIsPlaying(!isPlaying)}
						className="text-[#fbbf24] hover:text-white transition-colors ml-2"
					>
						{isPlaying ? (
							<Pause className="w-3.5 h-3.5" />
						) : (
							<Play className="w-3.5 h-3.5" />
						)}
					</button>
				</div>
			</div>

			<div
				ref={containerRef}
				className="flex-1 relative bg-[#050505] overflow-hidden"
			>
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.15]"
					style={{
						backgroundImage:
							"linear-gradient(#27272a 1px, transparent 1px), linear-gradient(90deg, #27272a 1px, transparent 1px)",
						backgroundSize: "40px 40px",
					}}
				/>

				{nodesRef.current.length === 0 ? (
					<div className="flex h-full w-full items-center justify-center font-mono text-xs text-[#52525b]">
						Waiting for impulse observations...
					</div>
				) : (
					<svg ref={svgRef} className="w-full h-full absolute inset-0" />
				)}

				{activeEvents.length > 0 && (
					<div className="absolute top-4 left-4 w-64 bg-[#09090b]/80 backdrop-blur border border-[#27272a] rounded p-3 pointer-events-none shadow-xl">
						<div className="text-[10px] uppercase tracking-widest text-[#52525b] mb-2 flex items-center justify-between">
							<span>Market Tape Feed</span>
							{isPlaying && (
								<span className="w-1.5 h-1.5 rounded-full bg-[#22c55e] animate-pulse" />
							)}
						</div>
						<div className="space-y-1.5 font-mono text-xs">
							{activeEvents.slice(0, 4).map((evt) => (
								<div
									key={evt.label}
									className="flex justify-between items-center text-[#d4d4d4]"
								>
									<span className="truncate">{evt.label}</span>
									<span className="text-[#fbbf24]">
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
