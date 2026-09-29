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

	// Categorical colors for real basin / cluster IDs
	const basinColors = useMemo(() => {
		return d3.scaleOrdinal<number, string>(d3.schemeTableau10);
	}, []);

	// Real coordinate projection without synthetic force layout
	const { projectedNodes, regionCentroids } = useMemo(() => {
		if (!data || data.length === 0) {
			return { projectedNodes: [], regionCentroids: [] };
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

		const pad = 40;
		const scaleX = d3
			.scaleLinear()
			.domain([minX, maxX])
			.range([pad, dimensions.width - pad]);
		const scaleY = d3
			.scaleLinear()
			.domain([minY, maxY])
			.range([dimensions.height - pad, pad]);

		const projected = data.map((n) => ({
			...n,
			projX: scaleX(n.x ?? 0),
			projY: scaleY(n.y ?? 0),
		}));

		// Compute centroid per region from member nodes
		const regionMap = new Map<number, { sumX: number; sumY: number; count: number }>();
		for (const n of projected) {
			const basinId = n.cluster ?? 0;
			const entry = regionMap.get(basinId) || { sumX: 0, sumY: 0, count: 0 };
			entry.sumX += n.projX;
			entry.sumY += n.projY;
			entry.count++;
			regionMap.set(basinId, entry);
		}

		const centroids = regions.map((r) => {
			const entry = regionMap.get(r.id);
			return {
				...r,
				x: entry && entry.count > 0 ? entry.sumX / entry.count : dimensions.width / 2,
				y: entry && entry.count > 0 ? entry.sumY / entry.count : dimensions.height / 2,
			};
		});

		return { projectedNodes: projected, regionCentroids: centroids };
	}, [data, regions, dimensions]);

	// Setup d3 zoom
	useEffect(() => {
		if (!svgRef.current) return;
		const svg = d3.select(svgRef.current);
		const g = svg.select<SVGGElement>("g.map-container");

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.5, 5])
			.on("zoom", (e) => {
				g.attr("transform", e.transform);
			});

		svg.call(zoom);
	}, []);

	return (
		<div className={`flex flex-col w-full h-full ${className ?? ""}`}>
			{/* Header Bar */}
			<div className="h-8 border-b border-(--line) flex items-center justify-between px-4 text-xs bg-(--surface)">
				<div className="flex gap-4 items-center">
					<span className="text-(--acc) border-b border-(--acc) py-1 font-bold">
						EMPIRICAL IMPULSE MAP
					</span>
					<span className="text-(--f3)">
						{data.length} cells · {regions.length} active regions
					</span>
				</div>
				<div className="flex items-center gap-3 text-[11px] text-(--f3)">
					<span>Learned coordinates (no synthetic physics)</span>
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
					<svg ref={svgRef} className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing">
						<title>Learned Impulse Map Topology</title>
						<g className="map-container">
							{/* Region Hotspots */}
							<g className="regions-layer" pointerEvents="none">
								{regionCentroids.map((r) => (
									<g key={r.id} transform={`translate(${r.x}, ${r.y})`}>
										<circle
											r={Math.max(12, Math.min(40, r.members * 4))}
											fill={basinColors(r.id)}
											fillOpacity={0.12}
											stroke={basinColors(r.id)}
											strokeWidth={1}
											strokeDasharray="3 3"
										/>
										<text
											y={-14}
											textAnchor="middle"
											fill={basinColors(r.id)}
											fontSize="9px"
											fontFamily="monospace"
											fontWeight="bold"
										>
											Region #{r.id} (str: {r.strength.toFixed(2)})
										</text>
									</g>
								))}
							</g>

							{/* Real Map Nodes */}
							<g className="nodes-layer">
								{projectedNodes.map((node) => {
									const isHovered = hoveredNode?.id === node.id;
									const baseRadius = Math.max(3, Math.min(8, (node.snr ?? 1) * 2 + 2));
									const basinColor = basinColors(node.cluster ?? 0);

									return (
										// biome-ignore lint/a11y/noStaticElementInteractions: SVG node point inspect hover
										<g
											key={node.id}
											transform={`translate(${node.projX}, ${node.projY})`}
											onMouseEnter={() => setHoveredNode(node)}
											onMouseLeave={() => setHoveredNode(null)}
											className="cursor-pointer"
										>
											{/* Activation pulse ring if active */}
											{(node.activation ?? 0) > 0 && (
												<circle
													r={baseRadius + 4}
													fill="none"
													stroke="var(--acc)"
													strokeWidth={1.5}
													strokeOpacity={Math.min(1, node.activation ?? 0)}
												/>
											)}
											<circle
												r={isHovered ? baseRadius + 2 : baseRadius}
												fill={basinColor}
												fillOpacity={node.present ? 0.9 : 0.25}
												stroke={isHovered ? "var(--acc)" : "var(--bg)"}
												strokeWidth={isHovered ? 2 : 1}
											/>
										</g>
									);
								})}
							</g>
						</g>
					</svg>
				)}

				{/* Hovered node inspection tooltip */}
				{hoveredNode && (
					<div className="absolute bottom-4 left-4 bg-(--surface)/90 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-xl text-[11px] font-mono z-10">
						<div className="text-(--f1) font-bold mb-1">{hoveredNode.label}</div>
						<div className="text-(--f3) flex flex-col gap-0.5">
							<span>Basin ID: <strong className="text-(--f1)">{hoveredNode.cluster}</strong></span>
							<span>Coordinates: ({hoveredNode.x?.toFixed(4)}, {hoveredNode.y?.toFixed(4)})</span>
							<span>Activity: {hoveredNode.activation?.toFixed(3)}</span>
							<span>SNR / Quality: {hoveredNode.snr?.toFixed(2)}</span>
						</div>
					</div>
				)}

				{/* Active Tape Feed */}
				{activeEvents.length > 0 && (
					<div className="absolute top-3 left-3 w-56 bg-(--surface)/85 backdrop-blur border border-(--line) rounded p-2.5 pointer-events-none shadow-lg">
						<div className="text-[10px] uppercase font-bold tracking-wider text-(--f4) mb-1.5 flex items-center justify-between">
							<span>Active Regions</span>
							<span className="w-1.5 h-1.5 rounded-full bg-(--up)" />
						</div>
						<div className="space-y-1 font-mono text-[10px]">
							{activeEvents.slice(0, 4).map((evt) => (
								<div key={evt.label} className="flex justify-between items-center text-(--f2)">
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
