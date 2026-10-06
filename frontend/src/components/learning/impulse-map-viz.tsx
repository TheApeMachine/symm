import * as d3 from "d3";
import { Grid2X2, Pause, Play, RefreshCw, Waves } from "lucide-react";
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
	/** Backend grid cell count (grid_cells); preferred over the node count. */
	gridCells?: number;
	/** Regions in the grid's last committed partition (grid_regions). */
	gridRegions?: number;
	className?: string;
}

export const ImpulseMapViz: React.FC<ImpulseMapVizProps> = ({
	data = [],
	regions: _regions = [],
	activeEvents = [],
	gridCells,
	gridRegions,
	className,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const svgRef = useRef<SVGSVGElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [isPlaying, setIsPlaying] = useState(true);
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");
	const [subTab, setSubTab] = useState<"map" | "topography" | "sympathy">("map");

	const simulationRef = useRef<d3.Simulation<ImpulseNode, undefined> | null>(null);
	const nodesRef = useRef<ImpulseNode[]>([]);
	const layoutModeRef = useRef<"grid" | "regions">(layoutMode);
	layoutModeRef.current = layoutMode;
	const isPlayingRef = useRef(true);
	isPlayingRef.current = isPlaying;

	const prevBoundsRef = useRef<{
		minX: number;
		maxX: number;
		minY: number;
		maxY: number;
		count: number;
		width: number;
		height: number;
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

	// Render scene function: updates contours, links, halos, nodes, and region badges.
	const renderScene = useCallback(
		(
			nodes: ImpulseNode[],
			mode: "grid" | "regions",
			currentWidth: number,
			currentHeight: number,
			_scale: number,
		) => {
			if (!svgRef.current || currentWidth === 0 || currentHeight === 0) return;

			const svg = d3.select(svgRef.current);
			const contourLayer = svg.select<SVGGElement>(".contours");
			const linkLayer = svg.select<SVGGElement>(".links");
			const haloLayer = svg.select<SVGGElement>(".halos");
			const nodeLayer = svg.select<SVGGElement>(".nodes");
			const badgeLayer = svg.select<SVGGElement>(".region-badges");

			// Color Scale: panel stone -> sky blue -> emerald green -> radiant gold
			const colorScale = d3
				.scaleSequential<string>((t) =>
					d3.interpolateRgbBasis(["#111113", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
				)
				.domain([0, 1]);

			// 1. Topographic Regions (Contours) in "regions" mode
			const hasActiveEnergy = nodes.some((d) => (d.activation || 0) > 0.05);
			if (mode === "regions" && nodes.length > 0 && hasActiveEnergy) {
				const computeDensity = d3
					.contourDensity<ImpulseNode>()
					.x((d) => d.x || 0)
					.y((d) => d.y || 0)
					.weight((d) => (d.activation || 0) * (d.snr || 1))
					.size([currentWidth, currentHeight])
					.bandwidth(30)
					.thresholds(15);

				const contourData = computeDensity(nodes);
				const paths = contourLayer.selectAll<SVGPathElement, d3.ContourMultiPolygon>("path").data(contourData);

				paths
					.enter()
					.append("path")
					.merge(paths)
					.attr("d", d3.geoPath())
					.attr("fill", (_, i) => {
						return d3.color("#fbbf24")?.copy({ opacity: i * 0.015 }).toString() || "none";
					})
					.attr("stroke", (_, i) => {
						if (i === 6) return "rgba(34, 197, 94, 0.4)"; // Green boundary
						if (i === 10) return "rgba(251, 191, 36, 0.8)"; // Gold Otsu split boundary
						return "none";
					})
					.attr("stroke-width", (_, i) => (i === 10 ? 1.5 : i === 6 ? 1 : 0))
					.style("opacity", 1);

				paths.exit().remove();
				contourLayer.style("opacity", 1);
			} else {
				contourLayer.selectAll("path").remove();
			}

			// 2. Sympathy Links: Connect active nodes in the same region
			const activeLinks: { source: ImpulseNode; target: ImpulseNode; strength: number }[] = [];
			const linkThreshold = 0.5;

			for (let i = 0; i < nodes.length; i++) {
				const a = nodes[i];
				if ((a.activation || 0) < linkThreshold) continue;
				for (let j = i + 1; j < nodes.length; j++) {
					const b = nodes[j];
					if ((b.activation || 0) < linkThreshold) continue;
					if (a.cluster !== b.cluster) continue;

					const dx = (a.x || 0) - (b.x || 0);
					const dy = (a.y || 0) - (b.y || 0);
					const dist = Math.hypot(dx, dy);

					if (dist < 100) {
						const strength = ((a.activation || 0) + (b.activation || 0)) / 2;
						activeLinks.push({ source: a, target: b, strength });
					}
				}
			}

			const links = linkLayer.selectAll<SVGLineElement, (typeof activeLinks)[0]>("line").data(activeLinks);

			links
				.enter()
				.append("line")
				.merge(links)
				.attr("x1", (d) => d.source.x || 0)
				.attr("y1", (d) => d.source.y || 0)
				.attr("x2", (d) => d.target.x || 0)
				.attr("y2", (d) => d.target.y || 0)
				.attr("stroke", "#fbbf24")
				.attr("stroke-opacity", (d) => Math.min(d.strength * 0.8, 0.8))
				.attr("stroke-width", 1.5);

			links.exit().remove();

			// 3. Halos for highly active nodes
			const activeNodes = nodes.filter((d) => (d.activation || 0) >= 0.5);
			const halos = haloLayer.selectAll<SVGCircleElement, ImpulseNode>("circle").data(activeNodes, (d) => d.id);

			halos
				.enter()
				.append("circle")
				.merge(halos)
				.attr("cx", (d) => d.x || 0)
				.attr("cy", (d) => d.y || 0)
				.attr("r", (d) => Math.max(3.5, Math.min((d.snr || 1) * 1.5 + 2, 14)) * 1.8)
				.attr("fill", "none")
				.attr("stroke", "#fbbf24")
				.attr("stroke-width", 1.5)
				.attr("stroke-opacity", (d) => Math.min((d.activation || 0) * 0.7, 0.8))
				.style("filter", "url(#impulse-glow)");

			halos.exit().remove();

			// 4. Nodes
			const nodeElements = nodeLayer.selectAll<SVGCircleElement, ImpulseNode>("circle").data(nodes, (d) => d.id);

			const nodeEnter = nodeElements
				.enter()
				.append("circle")
				.attr("r", (d) => Math.max(3.5, Math.min((d.snr || 1) * 1.5 + 2, 14)))
				.attr("stroke", (d) => ((d.activation || 0) >= 0.5 ? "#fbbf24" : "#050505"))
				.attr("stroke-width", (d) => ((d.activation || 0) >= 0.5 ? 2 : 1.5))
				.on("mouseover", function () {
					d3.select(this).attr("stroke", "#fbbf24").attr("stroke-width", 2);
				})
				.on("mouseout", function (_, d) {
					d3.select(this)
						.attr("stroke", (d.activation || 0) >= 0.5 ? "#fbbf24" : "#050505")
						.attr("stroke-width", (d.activation || 0) >= 0.5 ? 2 : 1.5);
				});

			nodeEnter
				.append("title")
				.text(
					(d) =>
						`${d.label}\nCluster: #${d.cluster}\nActivation: ${(d.activation || 0).toFixed(2)}\nSNR: ${(d.snr || 0).toFixed(2)}`,
				);

			const nodeMerged = nodeEnter.merge(nodeElements);

			nodeMerged
				.attr("cx", (d) => d.x || 0)
				.attr("cy", (d) => d.y || 0)
				.attr("r", (d) => Math.max(3.5, Math.min((d.snr || 1) * 1.5 + 2, 14)))
				.attr("fill", (d) => colorScale(d.activation || 0))
				.attr("stroke", (d) => ((d.activation || 0) >= 0.5 ? "#fbbf24" : "#050505"))
				.attr("stroke-width", (d) => ((d.activation || 0) >= 0.5 ? 2 : 1.5));

			nodeMerged.select("title").text(
				(d) =>
					`${d.label}\nCluster: #${d.cluster}\nActivation: ${(d.activation || 0).toFixed(2)}\nSNR: ${(d.snr || 0).toFixed(2)}`,
			);

			nodeElements.exit().remove();

			// 5. Cluster Centroid Badges in "regions" mode
			if (mode === "regions" && nodes.length > 0) {
				const clusterMap = new Map<number, { sumX: number; sumY: number; count: number; maxAct: number }>();
				for (const n of nodes) {
					const c = n.cluster || 0;
					const entry = clusterMap.get(c) || { sumX: 0, sumY: 0, count: 0, maxAct: 0 };
					entry.sumX += n.x || 0;
					entry.sumY += n.y || 0;
					entry.count++;
					entry.maxAct = Math.max(entry.maxAct, n.activation || 0);
					clusterMap.set(c, entry);
				}

				const badgeData = Array.from(clusterMap.entries()).map(([clusterId, bdata]) => ({
					clusterId,
					x: bdata.sumX / bdata.count,
					y: bdata.sumY / bdata.count,
					count: bdata.count,
					maxAct: bdata.maxAct,
				}));

				const badges = badgeLayer.selectAll<SVGGElement, (typeof badgeData)[0]>("g").data(badgeData, (d) => d.clusterId);

				const badgeEnter = badges.enter().append("g");
				badgeEnter
					.append("text")
					.attr("text-anchor", "middle")
					.attr("font-family", "monospace")
					.attr("font-size", "10px")
					.attr("font-weight", "600");

				const badgeMerged = badgeEnter.merge(badges);
				badgeMerged
					.attr("transform", (d) => `translate(${d.x}, ${d.y - 18})`)
					.select("text")
					.attr("fill", (d) => (d.maxAct >= 0.5 ? "#fbbf24" : "#71717a"))
					.text((d) => `R${d.clusterId} (${d.count}m)`);

				badges.exit().remove();
			} else {
				badgeLayer.selectAll("g").remove();
			}
		},
		[],
	);

	// Setup SVG DOM elements once or on dimension change
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0) return;

		const svg = d3.select(svgRef.current);
		svg.selectAll("*").remove();

		// Add filter definitions for glowing effects
		const defs = svg.append("defs");
		const filter = defs
			.append("filter")
			.attr("id", "impulse-glow")
			.attr("x", "-30%")
			.attr("y", "-30%")
			.attr("width", "160%")
			.attr("height", "160%");
		filter.append("feGaussianBlur").attr("stdDeviation", "4").attr("result", "blur");
		const feMerge = filter.append("feMerge");
		feMerge.append("feMergeNode").attr("in", "blur");
		feMerge.append("feMergeNode").attr("in", "SourceGraphic");

		const g = svg.append("g");
		g.append("g").attr("class", "contours");
		g.append("g").attr("class", "links");
		g.append("g").attr("class", "halos");
		g.append("g").attr("class", "nodes");
		g.append("g").attr("class", "region-badges");

		// Initialize D3 simulation
		const simulation = d3
			.forceSimulation(nodesRef.current)
			.alpha(0)
			.stop();

		simulation.on("tick", () => {
			const width = dimensions.width || 800;
			const height = dimensions.height || 600;

			// Decay activations naturally over time when feed is running
			if (isPlayingRef.current) {
				for (const n of nodesRef.current) {
					if ((n.activation || 0) > 0) {
						n.activation = Math.max(0, (n.activation || 0) - 0.002);
					}
				}
			}

			renderScene(nodesRef.current, layoutModeRef.current, width, height, 30);
		});

		simulationRef.current = simulation;

		return () => {
			simulation.stop();
		};
	}, [dimensions.width, dimensions.height, renderScene]);

	// Keep nodesRef in sync with incoming data, compute layout targets, and refit
	useEffect(() => {
		if (!data || data.length === 0) return;

		const width = dimensions.width || 800;
		const height = dimensions.height || 600;
		const pad = 0.1;
		const usableW = Math.max(width * (1 - 2 * pad), 50);
		const usableH = Math.max(height * (1 - 2 * pad), 50);
		const centerX = width / 2;
		const centerY = height / 2;

		let minX = Number.POSITIVE_INFINITY;
		let maxX = Number.NEGATIVE_INFINITY;
		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;

		for (const d of data) {
			const nx = d.x;
			const ny = d.y;
			if (nx !== undefined && ny !== undefined) {
				if (nx < minX) minX = nx;
				if (nx > maxX) maxX = nx;
				if (ny < minY) minY = ny;
				if (ny > maxY) maxY = ny;
			}
		}

		const hasExplicitCoords = minX !== Number.POSITIVE_INFINITY && (maxX > minX || maxY > minY);
		const spanX = maxX - minX;
		const spanY = maxY - minY;

		let scale = 1;
		if (hasExplicitCoords) {
			if (spanX > 0 && spanY > 0) {
				scale = Math.min(usableW / spanX, usableH / spanY);
			} else if (spanX > 0) {
				scale = usableW / spanX;
			} else {
				scale = usableH / spanY;
			}
		}

		const midX = hasExplicitCoords ? (minX + maxX) / 2 : 0;
		const midY = hasExplicitCoords ? (minY + maxY) / 2 : 0;

		const cols = Math.ceil(Math.sqrt(data.length * (usableW / usableH)));
		const rows = Math.ceil(data.length / Math.max(1, cols));
		const stepX = usableW / Math.max(1, cols - 1);
		const stepY = usableH / Math.max(1, rows - 1);
		const gridStartX = centerX - usableW / 2;
		const gridStartY = centerY - usableH / 2;

		const boundsChanged =
			!prevBoundsRef.current ||
			prevBoundsRef.current.minX !== minX ||
			prevBoundsRef.current.maxX !== maxX ||
			prevBoundsRef.current.minY !== minY ||
			prevBoundsRef.current.maxY !== maxY ||
			prevBoundsRef.current.count !== data.length ||
			prevBoundsRef.current.width !== width ||
			prevBoundsRef.current.height !== height;

		prevBoundsRef.current = {
			minX,
			maxX,
			minY,
			maxY,
			count: data.length,
			width,
			height,
		};

		const existingMap = new Map<string, ImpulseNode>();
		for (const n of nodesRef.current) {
			existingMap.set(n.id, n);
		}

		const updatedNodes: ImpulseNode[] = data.map((d, i) => {
			const existing = existingMap.get(d.id);
			let targetGridX = 0;
			let targetGridY = 0;

			if (hasExplicitCoords && d.x !== undefined && d.y !== undefined) {
				targetGridX = centerX + ((d.x - midX) * scale);
				targetGridY = centerY + ((d.y - midY) * scale);
			} else {
				targetGridX = gridStartX + (i % cols) * stepX;
				targetGridY = gridStartY + Math.floor(i / cols) * stepY;
			}

			if (existing) {
				existing.activation = d.activation;
				existing.snr = d.snr;
				existing.cluster = d.cluster;
				existing.present = d.present;
				existing.label = d.label;
				existing.value = d.value;
				existing.gridX = targetGridX;
				existing.gridY = targetGridY;

				if (layoutModeRef.current === "grid" || boundsChanged) {
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
			if (layoutModeRef.current === "grid") {
				simulationRef.current.alpha(0).stop();
			}
		}

		renderScene(updatedNodes, layoutModeRef.current, width, height, 30);
	}, [data, dimensions.width, dimensions.height, renderScene]);

	// Handle Mode Transitions (Grid vs Regions)
	useEffect(() => {
		const simulation = simulationRef.current;
		const width = dimensions.width || 800;
		const height = dimensions.height || 600;
		const nodes = nodesRef.current;
		if (!simulation || width === 0) return;

		const centerX = width / 2;
		const centerY = height / 2;

		const foci = [
			{ x: width * 0.28, y: height * 0.32 }, // Cluster 0
			{ x: width * 0.72, y: height * 0.32 }, // Cluster 1
			{ x: width * 0.28, y: height * 0.68 }, // Cluster 2
			{ x: width * 0.72, y: height * 0.68 }, // Cluster 3
			{ x: width * 0.50, y: height * 0.50 }, // Cluster 4
		];

		if (layoutMode === "grid") {
			simulation.alpha(0).stop();
			for (const n of nodes) {
				n.x = n.gridX ?? centerX;
				n.y = n.gridY ?? centerY;
				n.vx = 0;
				n.vy = 0;
			}
			renderScene(nodes, "grid", width, height, 30);
		} else {
			simulation
				.nodes(nodes)
				.force(
					"x",
					d3.forceX<ImpulseNode>((d) => foci[Math.abs(Number(d.cluster || 0)) % foci.length].x).strength(0.12),
				)
				.force(
					"y",
					d3.forceY<ImpulseNode>((d) => foci[Math.abs(Number(d.cluster || 0)) % foci.length].y).strength(0.12),
				)
				.force(
					"collide",
					d3
						.forceCollide<ImpulseNode>()
						.radius((d) => Math.max(4, (d.snr || 1) * 1.5 + 4))
						.iterations(2),
				)
				.force("charge", d3.forceManyBody().strength(-15))
				.alpha(0.6)
				.restart();

			window.setTimeout(() => {
				simulationRef.current?.alpha(0).stop();
				renderScene(nodesRef.current, "regions", width, height, 30);
			}, 500);
		}
	}, [layoutMode, dimensions.width, dimensions.height, renderScene]);

	const handleReset = useCallback(() => {
		const width = dimensions.width || 800;
		const height = dimensions.height || 600;
		for (const n of nodesRef.current) {
			n.x = n.gridX ?? n.x;
			n.y = n.gridY ?? n.y;
			n.vx = 0;
			n.vy = 0;
			n.activation = 0;
		}
		simulationRef.current?.alpha(0).stop();
		renderScene(nodesRef.current, layoutModeRef.current, width, height, 30);
	}, [dimensions.width, dimensions.height, renderScene]);

	// Count metrics and formed regions
	const metricCount =
		gridCells !== undefined && gridCells > 0
			? gridCells
			: data.length > 0
				? data.length
				: nodesRef.current.length;
	const uniqueClusters = new Set<number>();
	for (const node of data) {
		if (node.cluster != null && Number(node.cluster) > 0) {
			uniqueClusters.add(Number(node.cluster));
		}
	}
	for (const r of _regions) {
		if (r.id != null && Number(r.id) > 0) {
			uniqueClusters.add(Number(r.id));
		}
	}
	const regionCount =
		gridRegions !== undefined
			? gridRegions
			: _regions.length > 0
				? _regions.length
				: uniqueClusters.size;

	return (
		<div className={cn("flex flex-col w-full h-full", className)}>
			{/* Top Bar matching mockup styling */}
			<div className="h-8 border-b border-[#27272a] flex items-center justify-between px-4 text-xs bg-[#09090b] font-mono shrink-0">
				<div className="flex items-center gap-4">
					<div className="flex gap-4">
						<button
							type="button"
							onClick={() => setSubTab("map")}
							className={cn(
								"py-1.5 transition-colors cursor-pointer",
								subTab === "map"
									? "text-[#fbbf24] border-b border-[#fbbf24] font-medium"
									: "text-[#52525b] hover:text-[#a1a1aa]",
							)}
						>
							Map
						</button>
						<button
							type="button"
							onClick={() => setSubTab("topography")}
							className={cn(
								"py-1.5 transition-colors cursor-pointer",
								subTab === "topography"
									? "text-[#fbbf24] border-b border-[#fbbf24] font-medium"
									: "text-[#52525b] hover:text-[#a1a1aa]",
							)}
						>
							Topography
						</button>
						<button
							type="button"
							onClick={() => setSubTab("sympathy")}
							className={cn(
								"py-1.5 transition-colors cursor-pointer",
								subTab === "sympathy"
									? "text-[#fbbf24] border-b border-[#fbbf24] font-medium"
									: "text-[#52525b] hover:text-[#a1a1aa]",
							)}
						>
							Sympathy Grid
						</button>
					</div>

					<div className="h-3 w-px bg-[#27272a]" />

					{/* Metrics & Formed Regions telemetry counters */}
					<div className="flex items-center gap-3 text-[11px]">
						<div className="flex items-center gap-1.5 bg-[#111113] border border-[#27272a] px-2 py-0.5 rounded">
							<span className="text-[#71717a] uppercase text-[10px]">Metrics</span>
							<span data-l="grid-metric-count" className="text-white font-bold">{metricCount}</span>
						</div>
						<div className="flex items-center gap-1.5 bg-[#111113] border border-[#27272a] px-2 py-0.5 rounded">
							<span className="text-[#71717a] uppercase text-[10px]">Regions Formed</span>
							<span data-l="grid-region-count" className="text-[#fbbf24] font-bold">{regionCount}</span>
						</div>
					</div>
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
							<span>Sympathy Clustering</span>
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
					<button
						type="button"
						onClick={() => setIsPlaying(!isPlaying)}
						className="text-[#fbbf24] hover:text-white transition-colors ml-1 p-0.5 cursor-pointer"
						title={isPlaying ? "Pause Feed" : "Resume Feed"}
					>
						{isPlaying ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
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

				{/* Canvas HUD Pill: Metrics & Regions Indicator */}
				<div className="absolute bottom-3 left-4 flex items-center gap-3 font-mono text-[11px] text-[#71717a] pointer-events-none bg-[#09090b]/80 backdrop-blur px-2.5 py-1 rounded border border-[#27272a] shadow-lg">
					<span className="flex items-center gap-1.5">
						<span className="w-1.5 h-1.5 rounded-full bg-[#0ea5e9]" />
						<span>Metrics:</span>
						<span data-l="hud-metric-count" className="text-white font-bold">{metricCount}</span>
					</span>
					<span className="text-[#27272a]">·</span>
					<span className="flex items-center gap-1.5">
						<span className="w-1.5 h-1.5 rounded-full bg-[#fbbf24]" />
						<span>Formed Regions:</span>
						<span data-l="hud-region-count" className="text-[#fbbf24] font-bold">{regionCount}</span>
					</span>
				</div>

				{/* Market Tape / Active Regions Overlay */}
				<div className="absolute top-4 left-4 w-64 bg-[#09090b]/80 backdrop-blur border border-[#27272a] rounded p-3 pointer-events-none shadow-xl font-mono">
					<div className="text-[10px] uppercase tracking-widest text-[#52525b] mb-2 flex items-center justify-between">
						<span>{activeEvents.length > 0 ? `Active Regions (${activeEvents.length})` : "Market Tape Feed"}</span>
						{isPlaying && <span className="w-1.5 h-1.5 rounded-full bg-[#22c55e] animate-pulse" />}
					</div>
					<div className="space-y-1.5 text-xs max-h-60 overflow-y-auto">
						{activeEvents.length > 0 ? (
							activeEvents.map((evt, index) => (
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
							))
						) : (
							<div className="text-[#71717a] text-[11px] italic">
								Quiescent · Awaiting impulse
							</div>
						)}
					</div>
				</div>
			</div>
		</div>
	);
};
