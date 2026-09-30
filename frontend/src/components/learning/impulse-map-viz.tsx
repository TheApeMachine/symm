import * as d3 from "d3";
import { Grid2X2, RefreshCw, Waves } from "lucide-react";
import type React from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
		label: string;
		activity: number;
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
	const gRef = useRef<SVGGElement | null>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");
	const [hoveredNode, setHoveredNode] = useState<ImpulseNode | null>(null);

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

	// Color Scales - dark panel -> blue -> green -> gold
	const colorScale = useMemo(() => {
		return d3
			.scaleSequential<string>((t) =>
				d3.interpolateRgbBasis(["#18181b", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
			)
			.domain([0, 1]);
	}, []);

	// Setup D3 Zoom on the SVG container
	useEffect(() => {
		if (!svgRef.current) return;
		const svg = d3.select(svgRef.current);

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.5, 5])
			.on("zoom", (event) => {
				if (gRef.current) {
					d3.select(gRef.current).attr("transform", event.transform);
				}
			});

		svg.call(zoom);
	}, []);

	// Reset zoom transform
	const handleResetZoom = useCallback(() => {
		if (!svgRef.current) return;
		const svg = d3.select(svgRef.current);
		svg.transition().duration(500).call(d3.zoom<SVGSVGElement, unknown>().transform, d3.zoomIdentity);
	}, []);

	// Project real SOM Grid coordinates (node.x, node.y) and calculate honest empirical region boundaries
	const { nodesWithPositions, regionHulls } = useMemo(() => {
		const width = dimensions.width || 800;
		const height = dimensions.height || 600;

		if (data.length === 0) {
			return {
				nodesWithPositions: [],
				regionHulls: [],
			};
		}

		// Find the bounding box of real SOM coordinates from nomagique/store/grid.go
		let minX = Number.POSITIVE_INFINITY;
		let maxX = Number.NEGATIVE_INFINITY;
		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;

		for (const node of data) {
			const nx = node.x ?? 0;
			const ny = node.y ?? 0;
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

		const padX = Math.max(40, width * 0.08);
		const padY = Math.max(40, height * 0.08);
		const usableWidth = width - padX * 2;
		const usableHeight = height - padY * 2;
		const spanX = maxX - minX || 1;
		const spanY = maxY - minY || 1;

		// Map each node directly to its honest SOM coordinates
		const positioned = data.map((node) => {
			const nx = node.x ?? 0;
			const ny = node.y ?? 0;
			const posX = padX + ((nx - minX) / spanX) * usableWidth;
			const posY = padY + ((ny - minY) / spanY) * usableHeight;

			return {
				...node,
				posX,
				posY,
			};
		});

		// Compute empirical region boundaries directly from member nodes' real positions
		const regionGroups = new Map<number, Array<{ posX: number; posY: number }>>();
		for (const node of positioned) {
			if (node.cluster > 0) {
				const group = regionGroups.get(node.cluster) || [];
				group.push({ posX: node.posX, posY: node.posY });
				regionGroups.set(node.cluster, group);
			}
		}

		const hulls = Array.from(regionGroups.entries()).map(([regionId, members]) => {
			let sumX = 0;
			let sumY = 0;
			for (const m of members) {
				sumX += m.posX;
				sumY += m.posY;
			}
			const centroidX = sumX / members.length;
			const centroidY = sumY / members.length;

			// Radius encompasses all member nodes with a small margin
			let maxDist = 0;
			for (const m of members) {
				const dist = Math.hypot(m.posX - centroidX, m.posY - centroidY);
				if (dist > maxDist) maxDist = dist;
			}
			const radius = Math.max(24, maxDist + 16);

			return {
				regionId,
				centroidX,
				centroidY,
				radius,
				count: members.length,
			};
		});

		return {
			nodesWithPositions: positioned,
			regionHulls: hulls,
		};
	}, [data, dimensions.width, dimensions.height]);

	return (
		<div className={cn("flex flex-col w-full h-full bg-[#050505] text-(--f1)", className)}>
			{/* Header / Dual-View Tab Switcher Bar */}
			<div className="h-10 border-b border-(--line) flex items-center justify-between px-4 text-xs bg-(--surface)/90 backdrop-blur z-10 shrink-0 font-mono">
				<div className="flex items-center gap-3">
					<span className="text-(--f3) uppercase tracking-wider text-[10px] font-bold">
						Impulse Topology:
					</span>
					{/* Dual-View Tab Controls */}
					<div className="flex items-center gap-1 bg-(--sunken) border border-(--line) p-0.5 rounded">
						<button
							type="button"
							data-l="view-initial-grid"
							onClick={() => setLayoutMode("grid")}
							className={cn(
								"px-2.5 py-1 rounded text-xs transition-all flex items-center gap-1.5 font-medium cursor-pointer",
								layoutMode === "grid"
									? "bg-(--surface) text-(--acc) shadow-sm border border-(--line)"
									: "text-(--f4) hover:text-(--f2)",
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
								"px-2.5 py-1 rounded text-xs transition-all flex items-center gap-1.5 font-medium cursor-pointer",
								layoutMode === "regions"
									? "bg-(--surface) text-(--acc) shadow-sm border border-(--line)"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							<Waves className="w-3.5 h-3.5" />
							<span>Discovered Regions</span>
						</button>
					</div>
				</div>

				<div className="flex items-center gap-3">
					{regions.length > 0 && (
						<span className="text-[10px] text-(--f3)">
							{regions.length} regions
						</span>
					)}
					<div className="flex items-center gap-1.5 text-[10px] text-(--f4)">
						<span className="w-2 h-2 rounded-full bg-(--acc) opacity-80" />
						<span>{layoutMode === "grid" ? "Geometric Lattice" : "Discovered Regional Basins"}</span>
					</div>
					<div className="h-3 w-px bg-(--line)" />
					<button
						type="button"
						onClick={handleResetZoom}
						className="text-(--f4) hover:text-(--f1) transition-colors p-1 cursor-pointer"
						title="Reset Zoom / Pan"
					>
						<RefreshCw className="w-3.5 h-3.5" />
					</button>
				</div>
			</div>

			{/* Visualization Area */}
			<div ref={containerRef} className="flex-1 relative overflow-hidden">
				{/* Background Grid Accent */}
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.08]"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "32px 32px",
					}}
				/>

				<svg ref={svgRef} className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing">
					<title>Live SOM Impulse Map</title>
					<g ref={gRef}>
						{/* Discovered Regions Layer */}
						{layoutMode === "regions" && (
							<g className="regions-layer">
								{regionHulls.map((hull) => {
									const labelLetter = String.fromCharCode(65 + ((hull.regionId - 1) % 26));
									return (
										<g key={hull.regionId} className="transition-opacity duration-300">
											<circle
												cx={hull.centroidX}
												cy={hull.centroidY}
												r={hull.radius}
												fill="rgba(251, 191, 36, 0.04)"
												stroke="rgba(251, 191, 36, 0.25)"
												strokeWidth={1}
												strokeDasharray="4 3"
											/>
											<text
												x={hull.centroidX}
												y={hull.centroidY - hull.radius - 6}
												textAnchor="middle"
												fill="#fbbf24"
												fontSize={10}
												fontFamily="monospace"
												fontWeight="bold"
												opacity={0.85}
											>
												{`Region ${labelLetter} (#${hull.regionId}) · ${hull.count} metrics`}
											</text>
										</g>
									);
								})}
							</g>
						)}

						{/* Metric Nodes Layer */}
						<g className="nodes-layer">
							{nodesWithPositions.map((node) => {
								const radius = Math.max(3.5, (node.snr || 1) * 1.5 + 2);
								const fill = colorScale(node.activation);
								const isHovered = hoveredNode?.id === node.id;

								return (
									// biome-ignore lint/a11y/noStaticElementInteractions: SVG node inspection
									// biome-ignore lint/a11y/useKeyWithMouseEvents: SVG hover inspection
									<g
										key={node.id}
										className="cursor-pointer"
										onMouseOver={() => setHoveredNode(node)}
										onMouseOut={() => setHoveredNode(null)}
									>
										<circle
											cx={node.posX}
											cy={node.posY}
											r={radius}
											fill={fill}
											stroke={isHovered ? "#fbbf24" : "#09090b"}
											strokeWidth={isHovered ? 2.5 : 1.2}
											opacity={node.present ? 0.95 : 0.25}
											className="transition-all duration-150"
										>
											<title>{`${node.label} · Region ${node.cluster} · (${node.x ?? 0}, ${node.y ?? 0})`}</title>
										</circle>
									</g>
								);
							})}
						</g>
					</g>
				</svg>

				{/* Empty state when no live telemetry has arrived yet - ZERO FAKE DATA */}
				{(!data || data.length === 0) && (
					<div className="absolute inset-0 flex items-center justify-center text-(--f4) font-mono text-xs pointer-events-none">
						No numeric cells yet — waiting for tape telemetry
					</div>
				)}

				{/* Hovered node inspection tooltip */}
				{hoveredNode && (
					<div className="absolute bottom-4 left-4 bg-(--surface)/95 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-xl text-[11px] font-mono z-10">
						<div className="text-(--f1) font-bold mb-1 flex items-center gap-2">
							<span>{hoveredNode.label}</span>
							<span className="text-[10px] text-(--f4) font-normal">
								(ID: {hoveredNode.id})
							</span>
						</div>
						<div className="text-(--f3) flex flex-col gap-0.5">
							<span>
								Discovered Region:{" "}
								<strong className="text-(--acc)">
									{hoveredNode.cluster > 0
										? `${String.fromCharCode(65 + ((hoveredNode.cluster - 1) % 26))} (ID ${hoveredNode.cluster})`
										: "None"}
								</strong>
							</span>
							<span>
								Lattice Position: ({hoveredNode.x ?? 0}, {hoveredNode.y ?? 0})
							</span>
							<span>
								Activation:{" "}
								<strong className="text-(--f1)">
									{(hoveredNode.activation ?? 0).toFixed(4)}
								</strong>
							</span>
							<span>SNR Authority: {(hoveredNode.snr ?? 1).toFixed(2)}</span>
						</div>
					</div>
				)}

				{/* Active Regions Empirical Overlay */}
				{activeEvents.length > 0 && (
					<div className="absolute top-4 right-4 w-56 bg-(--surface)/90 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-lg z-10">
						<div className="text-[10px] uppercase font-bold tracking-wider text-(--f4) mb-1.5 flex items-center justify-between">
							<span>Active Precursor Regions</span>
							<span className="w-1.5 h-1.5 rounded-full bg-(--up) animate-pulse" />
						</div>
						<div className="space-y-1 font-mono text-[10px]">
							{activeEvents.slice(0, 4).map((evt) => (
								<div
									key={evt.label}
									className="flex justify-between items-center text-(--f2)"
								>
									<span className="truncate">{evt.label}</span>
									<span className="text-(--acc) font-bold">
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
