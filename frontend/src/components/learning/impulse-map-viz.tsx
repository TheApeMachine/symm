import * as d3 from "d3";
import type React from "react";
import { useEffect, useMemo, useRef, useState } from "react";
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
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
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

	// Activation heat map: dark neutral -> deep blue -> vivid green -> gold
	const colorScale = useMemo(() => {
		return d3
			.scaleSequential<string>((t) =>
				d3.interpolateRgbBasis(["#18181b", "#0ea5e9", "#22c55e", "#fbbf24"])(t),
			)
			.domain([0, 1]);
	}, []);

	// Categorical colors for basin IDs (for inactive node strokes or fallback)
	const basinColors = useMemo(() => {
		return d3.scaleOrdinal<number, string>(d3.schemeTableau10);
	}, []);

	// Real 1:1 aspect-ratio coordinate projection preserving manifold geometry
	const { projectedNodes, activeCentroids, links } = useMemo(() => {
		if (!data || data.length === 0) {
			return { projectedNodes: [], activeCentroids: [], links: [] };
		}

		let minX = Number.POSITIVE_INFINITY;
		let maxX = Number.NEGATIVE_INFINITY;
		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;

		for (const n of data) {
			const nx = n.x ?? 0;
			const ny = n.y ?? 0;
			if (nx < minX) minX = nx;
			if (nx > maxX) maxX = nx;
			if (ny < minY) minY = ny;
			if (ny > maxY) maxY = ny;
		}

		if (!Number.isFinite(minX) || minX === maxX) {
			minX = 0;
			maxX = 1;
		}
		if (!Number.isFinite(minY) || minY === maxY) {
			minY = 0;
			maxY = 1;
		}

		// Preserve square aspect ratio so coordinate topology is not distorted
		const pad = 48;
		const mapWidth = Math.max(100, dimensions.width - pad * 2);
		const mapHeight = Math.max(100, dimensions.height - pad * 2);
		const size = Math.min(mapWidth, mapHeight);
		const offsetX = (dimensions.width - size) / 2;
		const offsetY = (dimensions.height - size) / 2;

		const scaleX = d3
			.scaleLinear()
			.domain([minX, maxX])
			.range([offsetX, offsetX + size]);
		const scaleY = d3
			.scaleLinear()
			.domain([minY, maxY])
			.range([offsetY + size, offsetY]);

		const projected = data.map((n) => ({
			...n,
			projX: scaleX(n.x ?? 0),
			projY: scaleY(n.y ?? 0),
		}));

		// Compute centroid per basin from member nodes
		const regionMap = new Map<number, { sumX: number; sumY: number; count: number }>();
		for (const n of projected) {
			const basinId = n.cluster ?? 0;
			const entry = regionMap.get(basinId) || { sumX: 0, sumY: 0, count: 0 };
			entry.sumX += n.projX;
			entry.sumY += n.projY;
			entry.count++;
			regionMap.set(basinId, entry);
		}

		// Only show active centroids with non-zero strength to avoid cluttering with inactive basins
		const litRegions = regions.filter((r) => r.strength > 0);
		const centroids = litRegions.map((r) => {
			const entry = regionMap.get(r.id);
			return {
				...r,
				x: entry && entry.count > 0 ? entry.sumX / entry.count : dimensions.width / 2,
				y: entry && entry.count > 0 ? entry.sumY / entry.count : dimensions.height / 2,
			};
		});

		// Compute sympathy links between highly active co-basin cells
		const sympathyLinks: Array<{
			x1: number;
			y1: number;
			x2: number;
			y2: number;
			opacity: number;
		}> = [];
		const linkThreshold = 0.25;

		for (let i = 0; i < projected.length; i++) {
			const a = projected[i];
			if ((a.activation ?? 0) < linkThreshold) continue;

			for (let j = i + 1; j < projected.length; j++) {
				const b = projected[j];
				if ((b.activation ?? 0) < linkThreshold) continue;
				if (a.cluster !== b.cluster) continue; // Same sympathetic basin

				const dx = a.projX - b.projX;
				const dy = a.projY - b.projY;
				const distSq = dx * dx + dy * dy;

				if (distSq < 100 * 100) {
					sympathyLinks.push({
						x1: a.projX,
						y1: a.projY,
						x2: b.projX,
						y2: b.projY,
						opacity: Math.min(0.8, ((a.activation ?? 0) + (b.activation ?? 0)) / 2),
					});
				}
			}
		}

		return { projectedNodes: projected, activeCentroids: centroids, links: sympathyLinks };
	}, [data, regions, dimensions]);

	// Topographic density contours over the active manifold cells
	const contourData = useMemo(() => {
		if (!projectedNodes || projectedNodes.length === 0) return [];
		try {
			const computeDensity = d3
				.contourDensity<typeof projectedNodes[0]>()
				.x((d) => d.projX)
				.y((d) => d.projY)
				.weight((d) => (d.activation || 0.05) * ((d.snr || 1) + 1))
				.size([dimensions.width, dimensions.height])
				.bandwidth(32)
				.thresholds(12);
			return computeDensity(projectedNodes);
		} catch {
			return [];
		}
	}, [projectedNodes, dimensions]);

	// Setup d3 zoom
	useEffect(() => {
		if (!svgRef.current) return;
		const svg = d3.select(svgRef.current);
		const g = svg.select<SVGGElement>("g.map-container");

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.5, 6])
			.on("zoom", (e) => {
				g.attr("transform", e.transform);
			});

		svg.call(zoom);
	}, []);

	const geoPath = useMemo(() => d3.geoPath(), []);

	return (
		<div className={`flex flex-col w-full h-full ${className ?? ""}`}>
			{/* Header Bar */}
			<div className="h-8 border-b border-(--line) flex items-center justify-between px-4 text-xs bg-(--surface)">
				<div className="flex gap-4 items-center">
					<span className="text-(--acc) border-b border-(--acc) py-1 font-bold">
						EMPIRICAL IMPULSE MAP
					</span>
					<span className="text-(--f3)">
						{data.length} cells · {regions.length} watershed basins
					</span>
				</div>
				<div className="flex items-center gap-4 text-[11px] text-(--f3)">
					<div className="flex items-center gap-1.5">
						<span className="w-2 h-2 rounded-full bg-(--acc) opacity-80" />
						<span>Agent Reaction Boundary (Otsu Split)</span>
					</div>
					<div className="h-3 w-px bg-(--line)" />
					<span>Learned coordinate topology</span>
				</div>
			</div>

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

				{data.length === 0 ? (
					<div className="flex h-full w-full items-center justify-center font-mono text-xs text-(--f4)">
						Waiting for real impulse map observations...
					</div>
				) : (
					<svg
						ref={svgRef}
						className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing"
						role="img"
						aria-label="Learned Impulse Map Topology"
					>
						<title>Learned Impulse Map Topology</title>
						<g className="map-container">
							{/* Topographic Contour Elevation Layer */}
							<g className="contours-layer" pointerEvents="none">
								{contourData.map((contour, idx) => {
									const isHotBoundary = idx === 8;
									const isMidBoundary = idx === 5;
									const fillOpacity = Math.min(0.2, (idx + 1) * 0.015);
									const stroke = isHotBoundary
										? "rgba(251, 191, 36, 0.8)"
										: isMidBoundary
											? "rgba(34, 197, 94, 0.45)"
											: "none";
									const strokeWidth = isHotBoundary ? 1.5 : isMidBoundary ? 1 : 0;

									return (
										<path
											key={`contour-${contour.value}-${idx}`}
											d={geoPath(contour) ?? undefined}
											fill="var(--acc)"
											fillOpacity={fillOpacity}
											stroke={stroke}
											strokeWidth={strokeWidth}
										/>
									);
								})}
							</g>

							{/* Sympathy Links Layer */}
							<g className="links-layer" pointerEvents="none">
								{links.map((link, idx) => (
									<line
										key={`link-${link.x1}-${link.y1}-${idx}`}
										x1={link.x1}
										y1={link.y1}
										x2={link.x2}
										y2={link.y2}
										stroke="var(--acc)"
										strokeOpacity={link.opacity}
										strokeWidth={1.2}
									/>
								))}
							</g>

							{/* Active Basin Centroids (Lit Only) */}
							<g className="regions-layer" pointerEvents="none">
								{activeCentroids.map((r) => {
									const hexId = `0x${(BigInt(r.id) & 0xffffn).toString(16).padStart(4, "0")}`;
									const radius = Math.max(16, Math.min(48, r.members * 3));

									return (
										<g key={`centroid-${r.id}`} transform={`translate(${r.x}, ${r.y})`}>
											<circle
												r={radius}
												fill="none"
												stroke="var(--acc)"
												strokeWidth={1}
												strokeOpacity={Math.min(0.6, r.strength)}
												strokeDasharray="4 3"
											/>
											<text
												y={-radius - 4}
												textAnchor="middle"
												fill="var(--acc)"
												fontSize="10px"
												fontFamily="monospace"
												fontWeight="bold"
											>
												Basin [{hexId}] (str: {(r.strength * 100).toFixed(0)}%)
											</text>
										</g>
									);
								})}
							</g>

							{/* Manifold Cells Layer */}
							<g className="nodes-layer">
								{projectedNodes.map((node) => {
									const isHovered = hoveredNode?.id === node.id;
									const baseRadius = Math.max(
										3,
										Math.min(8, (node.snr ?? 1) * 1.8 + 2),
									);
									const nodeFill = colorScale(node.activation ?? 0);
									const basinColor = basinColors(node.cluster ?? 0);

									return (
										// biome-ignore lint/a11y/noStaticElementInteractions: SVG point inspection hover
										<g
											key={node.id}
											transform={`translate(${node.projX}, ${node.projY})`}
											onMouseEnter={() => setHoveredNode(node)}
											onMouseLeave={() => setHoveredNode(null)}
											className="cursor-pointer"
										>
											{/* Activation pulse ring if active */}
											{(node.activation ?? 0) > 0.1 && (
												<circle
													r={baseRadius + 5}
													fill="none"
													stroke="var(--acc)"
													strokeWidth={1.5}
													strokeOpacity={Math.min(0.9, node.activation ?? 0)}
												/>
											)}
											<circle
												r={isHovered ? baseRadius + 2.5 : baseRadius}
												fill={nodeFill}
												fillOpacity={node.present ? 0.95 : 0.25}
												stroke={isHovered ? "var(--acc)" : basinColor}
												strokeWidth={isHovered ? 2 : 1}
											/>
										</g>
									);
								})}
							</g>
						</g>
					</svg>
				)}

				{/* Hovered node inspection tooltip (Human provenance only per TRAINING.md Section 35) */}
				{hoveredNode && (
					<div className="absolute bottom-4 left-4 bg-(--surface)/95 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-xl text-[11px] font-mono z-10">
						<div className="text-(--f1) font-bold mb-1 flex items-center gap-2">
							<span>{hoveredNode.label}</span>
							<span className="text-[10px] text-(--f4) font-normal">
								(provenance)
							</span>
						</div>
						<div className="text-(--f3) flex flex-col gap-0.5">
							<span>
								Basin ID:{" "}
								<strong className="text-(--acc)">
									{hoveredNode.cluster}
								</strong>
							</span>
							<span>
								Coordinates: ({hoveredNode.x?.toFixed(4)},{" "}
								{hoveredNode.y?.toFixed(4)})
							</span>
							<span>
								Activity:{" "}
								<strong className="text-(--f1)">
									{hoveredNode.activation?.toFixed(3)}
								</strong>
							</span>
							<span>SNR / Quality: {hoveredNode.snr?.toFixed(2)}</span>
						</div>
					</div>
				)}

				{/* Active Basins Tape Overlay */}
				{activeEvents.length > 0 && (
					<div className="absolute top-3 left-3 w-60 bg-(--surface)/90 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-lg">
						<div className="text-[10px] uppercase font-bold tracking-wider text-(--f4) mb-1.5 flex items-center justify-between">
							<span>Active Basins (Empirical)</span>
							<span className="w-1.5 h-1.5 rounded-full bg-(--up) animate-pulse" />
						</div>
						<div className="space-y-1.5 font-mono text-[10px]">
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
