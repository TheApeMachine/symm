import * as d3 from "d3";
import { Grid2X2, RefreshCw, Waves } from "lucide-react";
import type React from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { cn } from "#/lib/utils";
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
		id?: number;
		label: string;
		activity: number;
		members?: number;
	}>;
	className?: string;
}

export const ImpulseMapViz: React.FC<ImpulseMapVizProps> = ({
	data = [],
	regions = [],
	activeEvents = [],
	className,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const svgRef = useRef<SVGSVGElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");

	// Refs for D3 simulation to persist across renders without triggering React state updates
	const simulationRef = useRef<d3.Simulation<ImpulseNode, undefined> | null>(null);
	const nodesRef = useRef<ImpulseNode[]>([]);
	const layoutModeRef = useRef<"grid" | "regions">(layoutMode);
	layoutModeRef.current = layoutMode;
	const settledCountRef = useRef(0);
	const fitLockedRef = useRef(false);
	const fitBoundsRef = useRef<{
		minX: number;
		maxX: number;
		minY: number;
		maxY: number;
	} | null>(null);

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

	// Keep nodesRef in sync with incoming data. Auto-fit while the cell census
	// is still growing, then lock the fit so the camera does not drift.
	useEffect(() => {
		if (!data || data.length === 0) return;

		const width = dimensions.width || 800;
		const height = dimensions.height || 600;

		let minX = Number.POSITIVE_INFINITY;
		let maxX = Number.NEGATIVE_INFINITY;
		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;

		for (const d of data) {
			const nx = d.x ?? 0;
			const ny = d.y ?? 0;
			if (nx < minX) minX = nx;
			if (nx > maxX) maxX = nx;
			if (ny < minY) minY = ny;
			if (ny > maxY) maxY = ny;
		}

		if (minX === Number.POSITIVE_INFINITY || maxX === minX) {
			minX = 0;
			maxX = Math.max(32, minX + 1);
		}
		if (minY === Number.POSITIVE_INFINITY || maxY === minY) {
			minY = 0;
			maxY = Math.max(32, minY + 1);
		}

		if (data.length > settledCountRef.current) {
			settledCountRef.current = data.length;
			fitLockedRef.current = false;
			fitBoundsRef.current = null;
		} else if (data.length >= settledCountRef.current && settledCountRef.current > 0) {
			fitLockedRef.current = true;
		}

		// Freeze the data→screen projection after the first settled fit so live
		// tape min/max churn cannot slide the grid under the cursor.
		if (!fitLockedRef.current || fitBoundsRef.current === null) {
			fitBoundsRef.current = { minX, maxX, minY, maxY };
		}
		const fit = fitBoundsRef.current;
		const spanX = fit.maxX - fit.minX || 1;
		const spanY = fit.maxY - fit.minY || 1;
		minX = fit.minX;
		maxX = fit.maxX;
		minY = fit.minY;
		maxY = fit.maxY;
		// Full-center fit: uniform scale (contain) + center in the viewport.
		// Independent X/Y stretch skewed lattice geometry and left empty margins.
		const pad = 0.06;
		const usableW = width * (1 - 2 * pad);
		const usableH = height * (1 - 2 * pad);
		const scale = Math.min(usableW / spanX, usableH / spanY);
		const offsetX = width * pad + (usableW - spanX * scale) / 2;
		const offsetY = height * pad + (usableH - spanY * scale) / 2;

		const existingMap = new Map<string, ImpulseNode>();
		for (const n of nodesRef.current) {
			existingMap.set(n.id, n);
		}

		const updatedNodes: ImpulseNode[] = data.map((d) => {
			const existing = existingMap.get(d.id);
			const targetGridX = offsetX + (((d.x ?? 0) - minX) * scale);
			const targetGridY = offsetY + (((d.y ?? 0) - minY) * scale);

			if (existing) {
				existing.activation = d.activation;
				existing.snr = d.snr;
				existing.cluster = d.cluster;
				existing.present = d.present;
				if (!fitLockedRef.current) {
					existing.gridX = targetGridX;
					existing.gridY = targetGridY;
					existing.x = targetGridX;
					existing.y = targetGridY;
					existing.vx = 0;
					existing.vy = 0;
				}
				return existing;
			}

			return {
				...d,
				gridX: targetGridX,
				gridY: targetGridY,
				x: targetGridX,
				y: targetGridY,
				vx: 0,
				vy: 0,
			};
		});

		nodesRef.current = updatedNodes;

		if (simulationRef.current) {
			simulationRef.current.nodes(updatedNodes);
			// Live tape must not reheat the layout — that was the visible drift.
			simulationRef.current.alpha(0).stop();
		}

		// Restyle without restarting forces so lit regions track precursor_tokens.
		if (svgRef.current) {
			const maxActivation = d3.max(updatedNodes, (d) => d.activation || 0) || 1;
			const colorScale = d3
				.scaleSequential<string>((t) =>
					d3.interpolateRgbBasis(["#111113", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
				)
				.domain([0, maxActivation]);
			d3.select(svgRef.current)
				.select(".nodes")
				.selectAll<SVGCircleElement, ImpulseNode>("circle")
				.data(updatedNodes, (d) => d.id)
				.join(
					(enter) =>
						enter
							.append("circle")
							.attr("r", (d) => (d.snr || 1) * 1.5 + 2)
							.attr("stroke", "#050505")
							.attr("stroke-width", 1.5)
							.attr("cx", (d) => d.x || 0)
							.attr("cy", (d) => d.y || 0),
					(update) => update,
					(exit) => exit.remove(),
				)
				.attr("fill", (d) => colorScale(d.activation || 0))
				.attr("cx", (d) => d.x || 0)
				.attr("cy", (d) => d.y || 0);
		}
	}, [data, dimensions.width, dimensions.height]);

	// Initialize D3 Visualization
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0) return;

		const svg = d3.select(svgRef.current);
		svg.selectAll("*").remove();

		const width = dimensions.width;
		const height = dimensions.height;

		// Static full-frame view: no zoom/pan. Fit is owned by the data→grid
		// projection so the map never drifts under the cursor or live tape.
		const g = svg.append("g");

		// Layer groups to ensure correct z-index
		const contourLayer = g.append("g").attr("class", "contours");
		const linkLayer = g.append("g").attr("class", "links");
		const nodeLayer = g.append("g").attr("class", "nodes");

		const nodes = nodesRef.current;

		// Color Scales: dark panel -> dim blue -> green -> gold
		const maxActivation = d3.max(nodes, (d) => d.activation || 0) || 1;
		const colorScale = d3
			.scaleSequential<string>((t) =>
				d3.interpolateRgbBasis(["#111113", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
			)
			.domain([0, maxActivation]);

		// Setup Initial Simulation
		const simulation = d3
			.forceSimulation(nodes)
			.force(
				"x",
				d3.forceX<ImpulseNode>((d) => d.gridX ?? width / 2).strength(1),
			)
			.force(
				"y",
				d3.forceY<ImpulseNode>((d) => d.gridY ?? height / 2).strength(1),
			)
			.force("collide", null)
			.force("charge", null)
			.force("center", null)
			.alpha(0)
			.alphaDecay(1)
			.stop();

		simulationRef.current = simulation;

		// Create node elements
		const nodeElements = nodeLayer
			.selectAll<SVGCircleElement, ImpulseNode>("circle")
			.data(nodes, (d) => d.id)
			.enter()
			.append("circle")
			.attr("r", (d) => (d.snr || 1) * 1.5 + 2)
			.attr("fill", (d) => colorScale(d.activation || 0))
			.attr("stroke", "#050505")
			.attr("stroke-width", 1.5)
			.on("mouseover", function () {
				d3.select(this).attr("stroke", "#fbbf24").attr("stroke-width", 2);
			})
			.on("mouseout", function () {
				d3.select(this).attr("stroke", "#050505").attr("stroke-width", 1.5);
			});

		// Contour Generator for Regions / Water Table
		const computeDensity = d3
			.contourDensity<ImpulseNode>()
			.x((d) => d.x || 0)
			.y((d) => d.y || 0)
			.weight((d) => (d.activation || 0.05) * (d.snr || 1))
			.size([width, height])
			.bandwidth(30)
			.thresholds(15);

		// Render loop
		simulation.on("tick", () => {
			// Update Node Positions & Colors
			nodeElements
				.attr("cx", (d) => d.x || 0)
				.attr("cy", (d) => d.y || 0)
				.attr("fill", (d) => colorScale(d.activation || 0));

			// Topographic Regions (Contours)
			if (layoutModeRef.current === "regions") {
				const contourData = computeDensity(nodes);
				const paths = contourLayer.selectAll("path").data(contourData);

				paths
					.enter()
					.append("path")
					.merge(paths as any)
					.attr("d", d3.geoPath())
					.attr("fill", (_, i) => {
						return (
							d3.color("#fbbf24")?.copy({ opacity: i * 0.015 }).toString() ||
							"none"
						);
					})
					.attr("stroke", (_, i) => {
						if (i === 6) return "rgba(34, 197, 94, 0.4)";
						if (i === 10) return "rgba(251, 191, 36, 0.8)";
						return "none";
					})
					.attr("stroke-width", (_, i) => (i === 10 ? 1.5 : i === 6 ? 1 : 0));

				paths.exit().remove();
			} else {
				contourLayer.selectAll("path").remove();
			}

			// Sympathy Links: Connect active nodes in the same region
			const activeLinks: { source: ImpulseNode; target: ImpulseNode }[] = [];
			const threshold = 0.5;

			for (let i = 0; i < nodes.length; i++) {
				if ((nodes[i].activation || 0) < threshold) continue;
				for (let j = i + 1; j < nodes.length; j++) {
					if ((nodes[j].activation || 0) < threshold) continue;
					if (nodes[i].cluster !== nodes[j].cluster) continue;

					const dx = (nodes[i].x || 0) - (nodes[j].x || 0);
					const dy = (nodes[i].y || 0) - (nodes[j].y || 0);
					const dist = Math.hypot(dx, dy);

					if (dist < 100) {
						activeLinks.push({ source: nodes[i], target: nodes[j] });
					}
				}
			}

			const links = linkLayer.selectAll("line").data(activeLinks);

			links
				.enter()
				.append("line")
				.merge(links as any)
				.attr("x1", (d) => d.source.x || 0)
				.attr("y1", (d) => d.source.y || 0)
				.attr("x2", (d) => d.target.x || 0)
				.attr("y2", (d) => d.target.y || 0)
				.attr("stroke", "#fbbf24")
				.attr(
					"stroke-opacity",
					(d) =>
						Math.min(
							((d.source.activation || 0) + (d.target.activation || 0)) / 2,
							0.8,
						),
				)
				.attr("stroke-width", 1.5);

			links.exit().remove();
		});

		return () => {
			simulation.stop();
		};
	}, [dimensions]);

	// Handle Mode Transitions (Grid vs Regions)
	useEffect(() => {
		const simulation = simulationRef.current;
		if (!simulation || dimensions.width === 0) return;

		const width = dimensions.width;
		const height = dimensions.height;
		const nodes = nodesRef.current;

		// Compute focal points for each real cluster
		const clusterCentroids = new Map<number, { sumX: number; sumY: number; count: number }>();
		for (const n of nodes) {
			const c = n.cluster || 0;
			const entry = clusterCentroids.get(c) || { sumX: 0, sumY: 0, count: 0 };
			entry.sumX += n.gridX ?? width / 2;
			entry.sumY += n.gridY ?? height / 2;
			entry.count++;
			clusterCentroids.set(c, entry);
		}

		const foci = new Map<number, { x: number; y: number }>();
		clusterCentroids.forEach((val, c) => {
			foci.set(c, { x: val.sumX / val.count, y: val.sumY / val.count });
		});

		if (layoutMode === "grid") {
			simulation
				.force(
					"x",
					d3.forceX<ImpulseNode>((d) => d.gridX ?? width / 2).strength(1),
				)
				.force(
					"y",
					d3.forceY<ImpulseNode>((d) => d.gridY ?? height / 2).strength(1),
				)
				.force("collide", null)
				.force("charge", null);

			d3.select(svgRef.current)
				.select(".contours")
				.style("opacity", 0);
		} else {
			simulation
				.force(
					"x",
					d3
						.forceX<ImpulseNode>(
							(d) => foci.get(d.cluster)?.x ?? d.gridX ?? width / 2,
						)
						.strength(0.08),
				)
				.force(
					"y",
					d3
						.forceY<ImpulseNode>(
							(d) => foci.get(d.cluster)?.y ?? d.gridY ?? height / 2,
						)
						.strength(0.08),
				)
				.force(
					"collide",
					d3
						.forceCollide<ImpulseNode>()
						.radius((d) => (d.snr || 1) * 1.5 + 2)
						.iterations(2),
				)
				.force("charge", d3.forceManyBody().strength(-8))
				.alphaDecay(0.08);

			d3.select(svgRef.current)
				.select(".contours")
				.transition()
				.duration(600)
				.style("opacity", 1);
		}

		if (layoutMode === "grid") {
			for (const n of nodes) {
				n.x = n.gridX ?? width / 2;
				n.y = n.gridY ?? height / 2;
				n.vx = 0;
				n.vy = 0;
			}
			simulation.alpha(0).stop();
		} else {
			// One short settle into region foci, then freeze — no continuous drift.
			simulation.alpha(0.35).restart();
			window.setTimeout(() => {
				simulationRef.current?.alpha(0).stop();
			}, 900);
		}
	}, [layoutMode, dimensions]);

	const handleReset = useCallback(() => {
		fitLockedRef.current = false;
		fitBoundsRef.current = null;
		settledCountRef.current = 0;
		for (const n of nodesRef.current) {
			n.x = n.gridX ?? n.x;
			n.y = n.gridY ?? n.y;
			n.vx = 0;
			n.vy = 0;
		}
		simulationRef.current?.alpha(0).stop();
	}, []);

	return (
		<div className={cn("flex flex-col w-full h-full", className)}>
			{/* Top Bar matching mockup styling */}
			<div className="h-8 border-b border-[#27272a] flex items-center justify-between px-4 text-xs bg-[#09090b] font-mono shrink-0">
				<div className="flex gap-4">
					<span className="text-[#fbbf24] border-b border-[#fbbf24] py-1.5 font-medium">
						Map
					</span>
					<span className="text-[#52525b] py-1.5">Topography</span>
					<span className="text-[#52525b] py-1.5">Sympathy Grid</span>
				</div>
				<div className="flex items-center gap-3">
					{/* Phase Toggle Controls */}
					<div className="flex items-center gap-1 bg-[#111113] border border-[#27272a] p-1 rounded mr-2">
						<button
							type="button"
							data-l="view-initial-grid"
							onClick={() => setLayoutMode("grid")}
							className={cn(
								"px-3 py-1 rounded transition-colors flex items-center gap-1.5 cursor-pointer text-xs",
								layoutMode === "grid"
									? "bg-[#27272a] text-[#fbbf24]"
									: "text-[#71717a] hover:text-[#a1a1aa]",
							)}
						>
							<Grid2X2 className="w-3.5 h-3.5" />
							<span>Initial Grid</span>
						</button>
						<button
							type="button"
							data-l="view-sympathy-regions"
							onClick={() => setLayoutMode("regions")}
							className={cn(
								"px-3 py-1 rounded transition-colors flex items-center gap-1.5 cursor-pointer text-xs",
								layoutMode === "regions"
									? "bg-[#27272a] text-[#fbbf24]"
									: "text-[#71717a] hover:text-[#a1a1aa]",
							)}
						>
							<Waves className="w-3.5 h-3.5" />
							<span>Discovered Regions</span>
						</button>
					</div>

					<div className="flex items-center gap-1.5 text-[#52525b] text-[11px]">
						<div className="w-2 h-2 rounded bg-[#fbbf24] opacity-80" />
						<span>Agent Action Threshold (Otsu Split)</span>
					</div>
					<div className="h-3 w-px bg-[#27272a]" />
					<button
						type="button"
						onClick={handleReset}
						className="text-[#71717a] hover:text-white transition-colors cursor-pointer p-0.5"
						title="Reset Map"
					>
						<RefreshCw className="w-3.5 h-3.5" />
					</button>
				</div>
			</div>

			<div ref={containerRef} className="flex-1 relative bg-[#050505] overflow-hidden">
				{/* Background Grid */}
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.15]"
					style={{
						backgroundImage:
							"linear-gradient(#27272a 1px, transparent 1px), linear-gradient(90deg, #27272a 1px, transparent 1px)",
						backgroundSize: "40px 40px",
					}}
				/>

				<svg
					ref={svgRef}
					className="w-full h-full absolute inset-0"
				>
					<title>Live SOM Impulse Map</title>
				</svg>

				{/* Active Regions Empirical Overlay */}
				{activeEvents.length > 0 && (
					<div className="absolute top-4 right-4 w-64 bg-[#09090b]/80 backdrop-blur border border-[#27272a] rounded p-3 pointer-events-none shadow-xl font-mono">
						<div className="text-[10px] uppercase tracking-widest text-[#52525b] mb-2 flex items-center justify-between">
							<span>Active Regions ({activeEvents.length})</span>
							<span className="w-1.5 h-1.5 rounded-full bg-[#22c55e] animate-pulse" />
						</div>
						<div className="space-y-1.5 text-xs max-h-60 overflow-y-auto">
							{activeEvents.map((evt, index) => (
								<div
									key={`${(evt as { id?: number }).id ?? "r"}:${evt.label}:${index}`}
									className="flex justify-between items-center text-[#d4d4d4]"
								>
									<span className="truncate pr-2">{evt.label}</span>
									<div className="flex items-center gap-1.5 shrink-0">
										{evt.members != null && (
											<span className="text-[10px] text-[#71717a]">
												{evt.members}m
											</span>
										)}
										<span className="text-[#fbbf24] font-bold">
											{evt.activity.toFixed(2)}
										</span>
									</div>
								</div>
							))}
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
