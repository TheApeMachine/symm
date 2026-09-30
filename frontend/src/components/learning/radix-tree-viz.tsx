import * as d3 from "d3";
import { SlidersHorizontal, Zap } from "lucide-react";
import { type MouseEvent, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import type { FeasibleAction, TrieNodeData } from "./types";

type TreeNode = d3.HierarchyPointNode<TrieNodeData>;
type TreeLink = d3.HierarchyPointLink<TrieNodeData>;

interface RadixTreeVizProps {
	data?: TrieNodeData | null;
	feasible?: FeasibleAction[];
	minProbability?: number;
	colorMode?: "threshold" | "gradient";
	projection?: "horizontal" | "vertical" | "radial";
	onProbabilityChange?: (prob: number) => void;
	onProjectionChange?: (proj: "horizontal" | "vertical" | "radial") => void;
	onColorModeChange?: (mode: "threshold" | "gradient") => void;
	className?: string;
}

const filterTree = (
	node: TrieNodeData,
	minProb: number,
): TrieNodeData | null => {
	if (node.probability < minProb) return null;
	const filteredNode = { ...node };

	if (filteredNode.children) {
		filteredNode.children = filteredNode.children
			.map((child) => filterTree(child, minProb))
			.filter((child): child is TrieNodeData => child !== null);
	}

	if (filteredNode._children) {
		filteredNode._children = filteredNode._children
			.map((child) => filterTree(child, minProb))
			.filter((child): child is TrieNodeData => child !== null);
	}

	return filteredNode;
};

export const RadixTreeViz: React.FC<RadixTreeVizProps> = ({
	data,
	feasible = [],
	minProbability: externalMinProb,
	colorMode: externalColorMode,
	projection: externalProjection,
	onProbabilityChange,
	onProjectionChange,
	onColorModeChange,
	className,
}) => {
	const svgRef = useRef<SVGSVGElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const zoomBehavior = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(
		null,
	);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [transform, setTransform] = useState<d3.ZoomTransform>(d3.zoomIdentity);

	const [internalMinProb, setInternalMinProb] = useState(0.0);
	const [internalColorMode, setInternalColorMode] = useState<
		"threshold" | "gradient"
	>("threshold");
	const [internalProjection, setInternalProjection] = useState<
		"horizontal" | "vertical" | "radial"
	>("horizontal");

	const minProbability = externalMinProb ?? internalMinProb;
	const colorMode = externalColorMode ?? internalColorMode;
	const projection = externalProjection ?? internalProjection;

	const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
	const [tooltip, setTooltip] = useState<{
		data: TrieNodeData;
		x: number;
		y: number;
	} | null>(null);

	const [treeData, setTreeData] = useState<TrieNodeData | null>(() =>
		data ? JSON.parse(JSON.stringify(data)) : null,
	);

	useEffect(() => {
		if (data) {
			setTreeData(JSON.parse(JSON.stringify(data)));
		} else {
			setTreeData(null);
		}
	}, [data]);

	useEffect(() => {
		const observeTarget = wrapperRef.current;
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

	const filteredTreeData = useMemo(() => {
		if (!treeData) return null;
		return filterTree(treeData, minProbability);
	}, [treeData, minProbability]);

	// Setup D3 Zoom
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0 || dimensions.height === 0)
			return;

		const svg = d3.select(svgRef.current);
		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.1, 4])
			.on("zoom", (e) => {
				setTransform(e.transform);
			});

		zoomBehavior.current = zoom;
		svg.call(zoom);

		const initialTransform = d3.zoomIdentity
			.translate(60, dimensions.height / 2)
			.scale(0.9);
		svg.call(zoom.transform, initialTransform);
	}, [dimensions.width, dimensions.height]);

	const handleNodeClick = (nodeData: TrieNodeData) => {
		if (!treeData) return;

		const toggleNode = (n: TrieNodeData): boolean => {
			if (n.id === nodeData.id) {
				if (n.children) {
					n._children = n.children;
					n.children = undefined;
				} else if (n._children) {
					n.children = n._children;
					n._children = undefined;
				}
				return true;
			}

			let found = false;
			if (n.children) {
				for (let i = 0; i < n.children.length; i++) {
					if (toggleNode(n.children[i])) found = true;
				}
			}
			if (n._children && !found) {
				for (let i = 0; i < n._children.length; i++) {
					if (toggleNode(n._children[i])) found = true;
				}
			}
			return found;
		};

		const newData = JSON.parse(JSON.stringify(treeData));
		toggleNode(newData);
		setTreeData(newData);
	};

	const focusBestPath = () => {
		if (!data) return;
		const newData: TrieNodeData = JSON.parse(JSON.stringify(data));

		const expandGreedyPath = (node: TrieNodeData) => {
			const allChildren = [...(node.children || []), ...(node._children || [])];
			if (allChildren.length === 0) {
				node.children = undefined;
				node._children = undefined;
				return;
			}

			const bestChild = allChildren.reduce(
				(max, child) => (child.probability > max.probability ? child : max),
				allChildren[0],
			);

			node.children = allChildren;
			node._children = undefined;

			for (const child of node.children) {
				if (child.id === bestChild.id) {
					expandGreedyPath(child);
				} else {
					const cAll = [...(child.children || []), ...(child._children || [])];
					if (cAll.length > 0) {
						child._children = cAll;
						child.children = undefined;
					}
				}
			}
		};

		expandGreedyPath(newData);
		setTreeData(newData);
	};

	const { nodes, links } = useMemo(() => {
		if (!filteredTreeData) return { nodes: [], links: [] };

		const root = d3.hierarchy<TrieNodeData>(filteredTreeData);
		const treeLayout = d3.tree<TrieNodeData>();

		if (projection === "radial") {
			let maxDepth = 0;
			root.each((d) => {
				if (d.depth > maxDepth) maxDepth = d.depth;
			});
			const radius = Math.max(300, maxDepth * 180);
			treeLayout.size([2 * Math.PI, radius]);
		} else if (projection === "vertical") {
			const nodeWidth = 120;
			const nodeHeight = 80;
			treeLayout.nodeSize([nodeWidth * 1.2, nodeHeight * 2]);
		} else {
			const nodeWidth = 140;
			const nodeHeight = 60;
			treeLayout.nodeSize([nodeHeight * 1.5, nodeWidth * 2.2]);
		}

		const pointRoot = treeLayout(root);

		return {
			nodes: pointRoot.descendants(),
			links: pointRoot.links(),
		};
	}, [filteredTreeData, projection]);

	const activePathIds = useMemo(() => {
		if (!hoveredNodeId) return null;
		const ids = new Set<string>();
		let current: TreeNode | null | undefined = nodes.find(
			(n) => n.data.id === hoveredNodeId,
		);
		while (current) {
			ids.add(current.data.id);
			current = current.parent;
		}
		return ids;
	}, [hoveredNodeId, nodes]);

	const getStateColor = (state?: string) => {
		if (state === "POLICY CHOICE") return "var(--up)";
		if (state === "EVALUATED") return "var(--acc)";
		if (state === "ESTIMATED") return "var(--info)";
		return "var(--f4)";
	};

	const getEdgeColor = (prob: number) => {
		if (colorMode === "gradient") {
			return d3.interpolateRgb("#3a342b", "#e8a33d")(prob) || "var(--line)";
		}
		if (prob > 0.3) return "var(--acc)";
		if (prob > 0.1) return "var(--warn)";
		return "var(--line2)";
	};

	const getNodePos = (node: TreeNode) => {
		const nx = node.x;
		const ny = node.y;
		if (projection === "radial") {
			const angle = nx - Math.PI / 2;
			return {
				x: ny * Math.cos(angle),
				y: ny * Math.sin(angle),
			};
		}
		if (projection === "vertical") {
			return { x: nx, y: ny };
		}
		return { x: ny, y: nx };
	};

	const getPath = (source: TreeNode, target: TreeNode) => {
		if (projection === "radial") {
			return (
				d3
					.linkRadial<TreeLink, TreeNode>()
					.angle((d: TreeNode) => d.x)
					.radius((d: TreeNode) => d.y)({ source, target }) ?? undefined
			);
		}
		if (projection === "vertical") {
			// Offset to connect exactly at the port boundaries
			const s = { x: source.x + 40, y: source.y + 15 };
			const t = { x: target.x + 40, y: target.y - 15 };
			return (
				d3
					// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman
					.linkVertical<any, { x: number; y: number }>()
					.x((d) => d.x)
					.y((d) => d.y)({ source: s, target: t }) ?? undefined
			);
		}

		// Horizontal projection: d.y is x-axis, d.x is y-axis
		// Source port is at cx: 90. Target port is at cx: -10
		const s = { x: source.x, y: source.y + 90 };
		const t = { x: target.x, y: target.y - 10 };
		return (
			d3
				// biome-ignore lint/suspicious/noExplicitAny: Because I'm Batman
				.linkHorizontal<any, { x: number; y: number }>()
				.x((d) => d.y)
				.y((d) => d.x)({ source: s, target: t }) ?? undefined
		);
	};

	return (
		<div
			className={cn(
				"flex flex-col w-full h-full bg-(--bg) text-(--f2) font-mono",
				className,
			)}
		>
			{/* Top Bar with Projection & Probability Controls */}
			<div className="h-8 border-(--line) border-b flex items-center justify-between px-3 text-xs bg-(--surface) shrink-0">
				<div className="flex gap-4 items-center">
					<span className="text-(--acc) border-(--acc) border-b py-1 text-[11px] font-bold tracking-wider">
						RADIX TRIE
					</span>
					<span className="text-(--f3) text-[11px]">
						{nodes.length} nodes · beam search topology
					</span>
				</div>

				<div className="flex items-center gap-3">
					{/* Projection selector */}
					<div className="flex items-center gap-1 bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px]">
						{(["horizontal", "vertical", "radial"] as const).map((mode) => (
							<button
								key={mode}
								type="button"
								onClick={() => {
									onProjectionChange
										? onProjectionChange(mode)
										: setInternalProjection(mode);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors capitalize",
									projection === mode
										? "bg-(--raised) text-(--acc) font-bold"
										: "text-(--f3) hover:text-(--f1)",
								)}
							>
								{mode}
							</button>
						))}
					</div>

					{/* Color mode selector */}
					<div className="flex items-center gap-1 bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px]">
						{(["threshold", "gradient"] as const).map((mode) => (
							<button
								key={mode}
								type="button"
								onClick={() => {
									onColorModeChange
										? onColorModeChange(mode)
										: setInternalColorMode(mode);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors capitalize",
									colorMode === mode
										? "bg-(--raised) text-(--acc) font-bold"
										: "text-(--f3) hover:text-(--f1)",
								)}
							>
								{mode}
							</button>
						))}
					</div>

					{/* Probability slider */}
					<div className="flex items-center gap-2 bg-(--sunken) border-(--line) border px-2 py-0.5 rounded text-[11px]">
						<SlidersHorizontal className="w-3 h-3 text-(--f3)" />
						<span className="text-(--f3)">Threshold:</span>
						<span className="text-(--acc) w-8 text-right font-bold">
							{(minProbability * 100).toFixed(0)}%
						</span>
						<input
							type="range"
							min="0"
							max="0.5"
							step="0.01"
							value={minProbability}
							onChange={(e) => {
								const val = parseFloat(e.target.value);
								onProbabilityChange
									? onProbabilityChange(val)
									: setInternalMinProb(val);
							}}
							className="w-16 accent-(--acc) bg-(--line) h-1 rounded appearance-none outline-none cursor-pointer"
						/>
					</div>
				</div>
			</div>

			{/* Center Visualizer Canvas */}
			<div
				ref={wrapperRef}
				className="flex-1 relative bg-(--sunken) overflow-hidden"
			>
				{/* Background Grid Pattern */}
				<div
					className="absolute inset-0 pointer-events-none opacity-[0.12]"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "24px 24px",
						transform: `translate(${transform.x % 24}px, ${transform.y % 24}px) scale(${transform.k})`,
						transformOrigin: "0 0",
					}}
				/>

				{!filteredTreeData && (
					<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-xs tracking-wider">
						Loading immutable radix trie state...
					</div>
				)}

				<svg
					ref={svgRef}
					className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing"
					role="img"
					aria-label="Radix trie graph visualization"
				>
					<title>Radix Trie Graph Visualization</title>
					<g transform={transform.toString()}>
						{/* Links Layer */}
						<g className="links">
							{links.map((link) => {
								const d = getPath(link.source, link.target);
								const sPos = getNodePos(link.source);
								const tPos = getNodePos(link.target);
								const midX = (sPos.x + tPos.x) / 2;
								const midY = (sPos.y + tPos.y) / 2;

								const isLinkActive = activePathIds
									? activePathIds.has(link.target.data.id)
									: true;

								return (
									<g
										key={`link-${link.source.data.id}-${link.target.data.id}`}
										style={{
											opacity: isLinkActive ? 1 : 0.15,
											transition: "opacity 0.3s ease-in-out",
										}}
									>
										<path
											d={d}
											fill="none"
											stroke={getEdgeColor(link.target.data.probability)}
											strokeWidth={Math.max(
												1.5,
												link.target.data.probability * 6,
											)}
											strokeOpacity={0.7}
											style={{
												transition: "d 0.3s ease-in-out",
											}}
										/>
										{link.target.data.tokens &&
											link.target.data.tokens.length > 0 && (
												<g
													transform={`translate(${midX}, ${projection === "vertical" ? midY - 6 : midY - 8})`}
													className="pointer-events-none select-none font-mono"
													style={{
														transition: "transform 0.3s ease-in-out",
													}}
												>
													<rect
														x={
															-Math.max(
																28,
																link.target.data.tokens.join(", ").length *
																	5.5 +
																	8,
															) / 2
														}
														y={-7}
														width={Math.max(
															28,
															link.target.data.tokens.join(", ").length * 5.5 +
																8,
														)}
														height={14}
														rx={2}
														fill="var(--surface)"
														stroke="var(--line2)"
														strokeWidth={1}
														opacity={0.95}
													/>
													<text
														x={0}
														y={3.5}
														fill="var(--acc)"
														fontSize="9px"
														textAnchor="middle"
														fontWeight="600"
														className="font-mono tracking-wider"
													>
														[{link.target.data.tokens.join(", ")}]
													</text>
												</g>
											)}
									</g>
								);
							})}
						</g>

						{/* Nodes Layer */}
						<g className="nodes">
							{nodes.map((node) => {
								const nodeData = node.data;
								const hasChildren = !!(nodeData.children || nodeData._children);
								const isCollapsed = !!nodeData._children;
								const probColor = getEdgeColor(nodeData.probability);
								const isHighProb = nodeData.probability > 0.2;

								const parent = node.parent;
								const pos = getNodePos(node);
								const isNodeActive = activePathIds
									? activePathIds.has(nodeData.id)
									: true;

								const parentPort =
									projection === "vertical"
										? { cx: 40, cy: -15 }
										: { cx: -10, cy: 0 };
								const childPort =
									projection === "vertical"
										? { cx: 40, cy: 15 }
										: { cx: 90, cy: 0 };

								return (
									// biome-ignore lint/a11y/noStaticElementInteractions: Because I'm Batman
									<g
										key={`node-${nodeData.id}`}
										style={{
											opacity: isNodeActive ? 1 : 0.15,
											transform: `translate(${pos.x}px, ${pos.y}px)`,
											transition: "all 0.3s ease-in-out",
										}}
										role={hasChildren ? "button" : "graphics-symbol"}
										tabIndex={hasChildren ? 0 : undefined}
										onKeyDown={(e) => {
											if (e.key === "Enter" || e.key === " ") {
												e.preventDefault();
												e.stopPropagation();
												if (hasChildren) handleNodeClick(nodeData);
											}
										}}
										onClick={(e: MouseEvent) => {
											e.stopPropagation();
											if (hasChildren) handleNodeClick(nodeData);
										}}
										onMouseEnter={(e: MouseEvent) => {
											setHoveredNodeId(nodeData.id);
											setTooltip({
												data: nodeData,
												x: e.clientX,
												y: e.clientY,
											});
										}}
										onMouseMove={(e: MouseEvent) => {
											setTooltip({
												data: nodeData,
												x: e.clientX,
												y: e.clientY,
											});
										}}
										onMouseLeave={() => {
											setHoveredNodeId(null);
											setTooltip(null);
										}}
										className={
											hasChildren ? "cursor-pointer" : "cursor-default"
										}
									>
										<g
											style={{
												transform:
													projection === "radial"
														? `rotate(${(node.x * 180) / Math.PI - 90}deg)`
														: "rotate(0deg)",
												transition: "transform 0.3s ease-in-out",
											}}
											className="origin-center"
										>
											<rect
												x={-10}
												y={-14}
												width={100}
												height={28}
												rx={3}
												fill={
													nodeData.prefix.toUpperCase() === "ENTER"
														? "rgba(115, 190, 104, 0.15)"
														: nodeData.prefix.toUpperCase() === "WAIT"
															? "rgba(232, 163, 61, 0.15)"
															: nodeData.prefix.toUpperCase() === "EXIT"
																? "rgba(240, 84, 79, 0.15)"
																: "var(--surface)"
												}
												stroke={
													nodeData.prefix.toUpperCase() === "ENTER"
														? "var(--up)"
														: nodeData.prefix.toUpperCase() === "WAIT"
															? "var(--warn)"
															: nodeData.prefix.toUpperCase() === "EXIT"
																? "var(--down)"
																: colorMode === "gradient"
																	? probColor
																	: isHighProb
																		? "var(--acc)"
																		: "var(--line2)"
												}
												strokeWidth={1.5}
												className={cn(
													"transition-colors",
													hasChildren && "hover:stroke-(--acc)",
												)}
											/>

											{parent && (
												<circle
													cx={parentPort.cx}
													cy={parentPort.cy}
													r={2.5}
													fill="var(--surface)"
													stroke="var(--f4)"
													strokeWidth={1.5}
												/>
											)}

											{(hasChildren || isCollapsed) && (
												<circle
													cx={childPort.cx}
													cy={childPort.cy}
													r={isCollapsed ? 3.5 : 2.5}
													fill={isCollapsed ? "var(--acc)" : "var(--surface)"}
													stroke={isCollapsed ? "var(--acc)" : "var(--f4)"}
													strokeWidth={1.5}
												/>
											)}

											<text
												x={0}
												y={0}
												dy="0.32em"
												fill={
													nodeData.prefix.toUpperCase() === "ENTER"
														? "var(--up)"
														: nodeData.prefix.toUpperCase() === "WAIT"
															? "var(--warn)"
															: nodeData.prefix.toUpperCase() === "EXIT"
																? "var(--down)"
																: "var(--f1)"
												}
												fontSize="11px"
												fontWeight="700"
												className="select-none pointer-events-none font-mono tracking-wider"
											>
												{nodeData.prefix}
											</text>

											<text
												x={80}
												y={-18}
												fill="var(--f4)"
												fontSize="9px"
												textAnchor="end"
												className="select-none pointer-events-none font-mono"
											>
												{(nodeData.probability * 100).toFixed(1)}%
											</text>

											{nodeData.state && (
												<text
													x={0}
													y={-18}
													fill={getStateColor(nodeData.state)}
													fontSize="8px"
													className="font-mono tracking-widest pointer-events-none select-none uppercase font-bold"
												>
													{nodeData.state}
												</text>
											)}
										</g>
									</g>
								);
							})}
						</g>
					</g>
				</svg>

				{/* Floating Tooltip */}
				{tooltip && (
					<div
						className="fixed z-50 bg-(--surface) border-(--line2) border rounded shadow-2xl p-2.5 text-[11px] pointer-events-none flex flex-col gap-1.5 font-mono"
						style={{
							left: tooltip.x + 15,
							top: tooltip.y + 15,
							minWidth: 200,
						}}
					>
						<div className="flex items-center gap-2 border-(--line) border-b pb-1.5 font-bold">
							<span className="text-(--acc)">{tooltip.data.prefix}</span>
							{tooltip.data.state && (
								<span
									className="ml-auto text-[8px] border border-current px-1 py-0.5 rounded uppercase tracking-wider"
									style={{ color: getStateColor(tooltip.data.state) }}
								>
									{tooltip.data.state}
								</span>
							)}
						</div>
						<div className="flex justify-between text-(--f3)">
							<span>Sequence Prob:</span>
							<span className="text-(--f1) font-bold">
								{(tooltip.data.probability * 100).toFixed(2)}%
							</span>
						</div>
						{tooltip.data.stepProbability !== undefined && (
							<div className="flex justify-between text-(--f3)">
								<span>Step Prob:</span>
								<span className="text-(--f1)">
									{(tooltip.data.stepProbability * 100).toFixed(2)}%
								</span>
							</div>
						)}
						{tooltip.data.tokens && tooltip.data.tokens.length > 0 && (
							<div className="flex justify-between mt-1 pt-1 border-(--line) border-t items-center text-(--f3)">
								<span>Tokens:</span>
								<span className="text-(--acc) bg-(--sunken) px-1 py-0.5 rounded font-bold">
									[{tooltip.data.tokens.join(", ")}]
								</span>
							</div>
						)}
					</div>
				)}

				{/* Canvas Controls */}
				<div className="absolute bottom-3 right-3 flex gap-2">
					<button
						type="button"
						onClick={focusBestPath}
						title="Expand Greedy Policy Beam"
						className="bg-(--surface) border-(--line) border text-(--acc) hover:bg-(--raised) px-2.5 py-1 rounded transition-colors text-[10px] font-bold tracking-wider flex items-center gap-1 shadow-lg"
					>
						<Zap className="w-3 h-3" />
						BEST BEAM
					</button>
					<button
						type="button"
						onClick={() => {
							if (svgRef.current && zoomBehavior.current) {
								d3.select(svgRef.current)
									.transition()
									.duration(300)
									.call(zoomBehavior.current.scaleBy, 1.25);
							}
						}}
						className="bg-(--surface) border-(--line) border text-(--f2) hover:text-(--acc) p-1 rounded transition-colors shadow-lg"
						title="Zoom in"
					>
						<svg
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							strokeWidth="2"
							role="img"
							aria-label="Zoom in"
						>
							<title>Zoom in</title>
							<path d="M12 5v14M5 12h14" />
						</svg>
					</button>
					<button
						type="button"
						onClick={() => {
							if (svgRef.current && zoomBehavior.current) {
								d3.select(svgRef.current)
									.transition()
									.duration(300)
									.call(zoomBehavior.current.scaleBy, 0.8);
							}
						}}
						className="bg-(--surface) border-(--line) border text-(--f2) hover:text-(--acc) p-1 rounded transition-colors shadow-lg"
						title="Zoom out"
					>
						<svg
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							strokeWidth="2"
							role="img"
							aria-label="Zoom out"
						>
							<title>Zoom out</title>
							<path d="M5 12h14" />
						</svg>
					</button>
				</div>
			</div>

			{/* Feasible Actions Bottom Table */}
			{feasible.length > 0 && (
				<div className="h-44 shrink-0 bg-(--surface) border-(--line) border-t flex flex-col">
					<div className="h-7 border-(--line) border-b flex items-center px-3 text-[10px] text-(--f3) uppercase tracking-widest font-bold bg-(--sunken)/40">
						Feasible actions at current impulse
					</div>
					<div className="flex-1 overflow-auto">
						<table className="w-full text-left text-[11px]">
							<thead className="text-(--f4) border-(--line) border-b bg-(--surface)">
								<tr>
									<th className="font-normal px-3 py-1.5 w-12">Rank</th>
									<th className="font-normal px-3 py-1.5 w-32">Action</th>
									<th className="font-normal px-3 py-1.5">Prefix Sequence</th>
									<th className="font-normal px-3 py-1.5 text-right">
										Probability
									</th>
									<th className="font-normal px-3 py-1.5 text-right">State</th>
								</tr>
							</thead>
							<tbody className="divide-y divide-(--line)">
								{feasible.map((act) => (
									<tr
										key={`${act.rank}-${act.prefix}`}
										className="hover:bg-(--raised) transition-colors cursor-default"
									>
										<td className="px-3 py-1.5 text-(--acc) font-bold">
											{act.rank}
										</td>
										<td className="px-3 py-1.5 text-(--f1) font-semibold">
											{act.action}
										</td>
										<td className="px-3 py-1.5 text-(--f3) font-mono">
											{act.prefix}
										</td>
										<td className="px-3 py-1.5 text-right text-(--f1) font-bold">
											{(act.probability * 100).toFixed(1)}%
										</td>
										<td className="px-3 py-1.5 text-right">
											<span
												className={cn(
													"border px-1.5 py-0.5 rounded text-[9px] uppercase tracking-wider font-bold",
													act.state === "POLICY CHOICE"
														? "border-(--up)/40 text-(--up)"
														: act.state === "EVALUATED"
															? "border-(--acc)/40 text-(--acc)"
															: "border-(--line2) text-(--f4)",
												)}
											>
												{act.state}
											</span>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				</div>
			)}
		</div>
	);
};
