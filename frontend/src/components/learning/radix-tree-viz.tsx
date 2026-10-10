import * as d3 from "d3";
import { type MouseEvent, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import type { FeasibleAction, SymbolProgress, TrieNodeData } from "./types";

type TreeNode = d3.HierarchyPointNode<TrieNodeData>;
type TreeLink = d3.HierarchyPointLink<TrieNodeData>;

interface RadixTreeVizProps {
	data?: TrieNodeData | null;
	keys?: string[] | null;
	symbols?: Record<string, SymbolProgress> | null;
	feasible?: FeasibleAction[] | null;
	minProbability?: number;
	colorMode?: "threshold" | "gradient";
	projection?: "horizontal" | "vertical" | "radial";
	onProbabilityChange?: (prob: number) => void;
	onProjectionChange?: (proj: "horizontal" | "vertical" | "radial") => void;
	onColorModeChange?: (mode: "threshold" | "gradient") => void;
	className?: string;
}

const INBETWEEN_HEIGHT = 24;
const INBETWEEN_WIDTH = INBETWEEN_HEIGHT;
const TERMINAL_WIDTH = 64;
const TERMINAL_HEIGHT = 26;
const ROOT_WIDTH = 44;
const ROOT_HEIGHT = 24;

const isNodeRoot = (node: TreeNode) => node.data.id === "root";
const isNodeTerminal = (node: TreeNode) =>
	!node.data.children?.length && !node.data._children?.length;

const getPorts = (
	node: TreeNode,
	projection: "horizontal" | "vertical" | "radial",
) => {
	const isRoot = isNodeRoot(node);
	const isTerminal = isNodeTerminal(node);

	if (projection === "vertical") {
		const halfH = isRoot
			? ROOT_HEIGHT / 2
			: isTerminal
				? TERMINAL_HEIGHT / 2
				: INBETWEEN_HEIGHT / 2;
		return {
			parentPort: isRoot ? null : { cx: 0, cy: -halfH },
			childPort: isTerminal ? null : { cx: 0, cy: halfH },
		};
	}

	// horizontal
	const halfW = isRoot
		? ROOT_WIDTH / 2
		: isTerminal
			? TERMINAL_WIDTH / 2
			: INBETWEEN_WIDTH / 2;
	return {
		parentPort: isRoot ? null : { cx: -halfW, cy: 0 },
		childPort: isTerminal ? null : { cx: halfW, cy: 0 },
	};
};

function buildTreeFromPrefixes(prefixes: string[]): TrieNodeData | null {
	if (!prefixes || prefixes.length === 0) return null;

	const root: TrieNodeData = {
		id: "root",
		prefix: "root",
		token: "",
		children: [],
	};

	let nextId = 1;

	for (const key of prefixes) {
		const parts = key.split("/").filter(Boolean);
		if (parts.length === 0) continue;

		const lastPart = parts[parts.length - 1];
		const isAction = lastPart.endsWith(".json");
		const action = isAction
			? lastPart.replace(/\.json$/i, "").toUpperCase()
			: null;
		const regionTokens = isAction ? parts.slice(0, -1) : parts;
		if (regionTokens.length === 0) continue;

		let current = root;
		let prefix = "";

		for (const token of regionTokens) {
			prefix = prefix ? `${prefix}/${token}` : token;
			if (!current.children) current.children = [];
			let child = current.children.find((c) => c.token === token);
			if (!child) {
				child = {
					id: String(nextId++),
					prefix,
					token,
					children: [],
				};
				current.children.push(child);
			}
			current = child;
		}

		if (action) {
			if (!current.action || current.action === "NOOP") {
				current.action = action;
			}
		}
	}

	return root;
}

function extractPrefixes(data?: TrieNodeData | null): string[] {
	if (!data) return [];
	const paths: string[] = [];

	const walk = (node: TrieNodeData, currentPath: string[]) => {
		const nextPath = node.token ? [...currentPath, node.token] : currentPath;
		if (node.action && nextPath.length > 0) {
			paths.push(`${nextPath.join("/")}/${node.action.toLowerCase()}.json`);
		}
		const children = [...(node.children || []), ...(node._children || [])];
		for (const child of children) {
			walk(child, nextPath);
		}
	};

	walk(data, []);
	return paths;
}

export const RadixTreeViz: React.FC<RadixTreeVizProps> = ({
	data,
	keys,
	symbols,
	projection: externalProjection,
	onProjectionChange,
	className,
}) => {
	const svgRef = useRef<SVGSVGElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const zoomBehavior = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(
		null,
	);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [transform, setTransform] = useState<d3.ZoomTransform>(d3.zoomIdentity);
	const [internalProjection, setInternalProjection] = useState<
		"horizontal" | "vertical" | "radial"
	>("horizontal");

	const projection = externalProjection ?? internalProjection;
	const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);

	const activeSymbolList = useMemo(() => {
		if (!symbols) return [];
		return Object.values(symbols).filter(
			(s): s is SymbolProgress => !!s && !!s.path && s.depth > 0,
		);
	}, [symbols]);

	// Set of all prefix paths traversed by active symbols
	const activePathPrefixes = useMemo(() => {
		const set = new Set<string>();
		const list = selectedSymbol
			? activeSymbolList.filter((s) => s.symbol === selectedSymbol)
			: activeSymbolList;

		for (const item of list) {
			let prefix = "";
			for (const tok of item.tokens) {
				prefix = prefix ? `${prefix}/${tok}` : tok;
				set.add(prefix);
			}
		}
		return set;
	}, [activeSymbolList, selectedSymbol]);

	// Map of exact prefix to symbols currently located at that node
	const symbolPositions = useMemo(() => {
		const map = new Map<string, SymbolProgress[]>();
		const list = selectedSymbol
			? activeSymbolList.filter((s) => s.symbol === selectedSymbol)
			: activeSymbolList;

		for (const item of list) {
			const existing = map.get(item.path) || [];
			existing.push(item);
			map.set(item.path, existing);
		}
		return map;
	}, [activeSymbolList, selectedSymbol]);

	const treeData = useMemo(() => {
		if (keys && keys.length > 0) {
			return buildTreeFromPrefixes(keys);
		}
		if (data) {
			const extracted = extractPrefixes(data);
			if (extracted.length > 0) {
				return buildTreeFromPrefixes(extracted);
			}
			return data;
		}
		return null;
	}, [data, keys]);

	const [collapsedIds, setCollapsedIds] = useState<Set<string>>(new Set());

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
			.translate(80, dimensions.height / 2)
			.scale(0.85);
		svg.call(zoom.transform, initialTransform);
	}, [dimensions.width, dimensions.height]);

	const handleNodeClick = (nodeData: TrieNodeData) => {
		setCollapsedIds((prev) => {
			const next = new Set(prev);
			if (next.has(nodeData.id)) {
				next.delete(nodeData.id);
			} else {
				next.add(nodeData.id);
			}
			return next;
		});
	};

	const { nodes, links } = useMemo(() => {
		if (!treeData) return { nodes: [], links: [] };

		// Clone and apply collapsing
		const prepareHierarchy = (n: TrieNodeData): TrieNodeData => {
			const isCollapsed = collapsedIds.has(n.id);
			const copy: TrieNodeData = { ...n };
			if (n.children && n.children.length > 0) {
				if (isCollapsed) {
					copy._children = n.children;
					copy.children = undefined;
				} else {
					copy.children = n.children.map(prepareHierarchy);
				}
			}
			return copy;
		};

		const displayTree = prepareHierarchy(treeData);
		const root = d3.hierarchy<TrieNodeData>(displayTree);
		const treeLayout = d3.tree<TrieNodeData>();

		if (projection === "radial") {
			let maxDepth = 0;
			root.each((d) => {
				if (d.depth > maxDepth) maxDepth = d.depth;
			});
			const radius = Math.max(300, maxDepth * 160);
			treeLayout.size([2 * Math.PI, radius]);
		} else if (projection === "vertical") {
			treeLayout.nodeSize([80, 110]);
		} else {
			// Horizontal: x is vertical step, y is horizontal step
			treeLayout.nodeSize([44, 140]);
		}

		const pointRoot = treeLayout(root);

		return {
			nodes: pointRoot.descendants(),
			links: pointRoot.links(),
		};
	}, [treeData, collapsedIds, projection]);

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

	const getLinkPorts = (source: TreeNode, target: TreeNode) => {
		const sPos = getNodePos(source);
		const tPos = getNodePos(target);
		const sPorts = getPorts(source, projection);
		const tPorts = getPorts(target, projection);

		if (projection === "vertical") {
			return {
				s: {
					x: sPos.x + (sPorts.childPort?.cx ?? 0),
					y: sPos.y + (sPorts.childPort?.cy ?? INBETWEEN_HEIGHT / 2),
				},
				t: {
					x: tPos.x + (tPorts.parentPort?.cx ?? 0),
					y: tPos.y + (tPorts.parentPort?.cy ?? -INBETWEEN_HEIGHT / 2),
				},
			};
		}

		// Horizontal projection
		return {
			s: {
				x: sPos.x + (sPorts.childPort?.cx ?? INBETWEEN_WIDTH / 2),
				y: sPos.y + (sPorts.childPort?.cy ?? 0),
			},
			t: {
				x: tPos.x + (tPorts.parentPort?.cx ?? -INBETWEEN_WIDTH / 2),
				y: tPos.y + (tPorts.parentPort?.cy ?? 0),
			},
		};
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

		const { s, t } = getLinkPorts(source, target);

		if (projection === "vertical") {
			return (
				d3
					// biome-ignore lint/suspicious/noExplicitAny: d3 vertical link typing
					.linkVertical<any, { x: number; y: number }>()
					.x((d) => d.x)
					.y((d) => d.y)({ source: s, target: t }) ?? undefined
			);
		}

		// Horizontal projection
		return (
			d3
				// biome-ignore lint/suspicious/noExplicitAny: d3 horizontal link typing
				.linkHorizontal<any, { x: number; y: number }>()
				.x((d) => d.x)
				.y((d) => d.y)({ source: s, target: t }) ?? undefined
		);
	};

	return (
		<div
			className={cn(
				"flex flex-col w-full h-full bg-(--bg) text-(--f2) font-mono",
				className,
			)}
		>
			{/* Top Bar with Projection Controls and Active Symbol Depth Ticker */}
			<div className="border-(--line) border-b flex flex-col bg-(--surface) shrink-0">
				<div className="h-8 flex items-center justify-between px-3 text-xs">
					<div className="flex gap-4 items-center">
						<span className="text-(--acc) border-(--acc) border-b py-1 text-[11px] font-bold tracking-wider">
							TRIE
						</span>
						<span className="text-(--f3) text-[11px]">
							{nodes.length} nodes
						</span>
						{activeSymbolList.length > 0 && (
							<span className="text-[#38bdf8] text-[11px] font-medium flex items-center gap-1.5">
								<span className="w-1.5 h-1.5 rounded-full bg-[#38bdf8] animate-pulse" />
								{activeSymbolList.length} symbols active in Step
							</span>
						)}
					</div>

					<div className="flex items-center gap-3">
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
					</div>
				</div>

				{/* Live Symbols Traversal Depth Strip */}
				{activeSymbolList.length > 0 && (
					<div className="flex items-center gap-1.5 px-3 py-1.5 border-t border-(--line)/50 overflow-x-auto text-[10px]">
						<span className="text-(--f4) uppercase tracking-wider text-[9px] font-semibold shrink-0">
							Depth:
						</span>
						{activeSymbolList.map((s) => {
							const isSelected = selectedSymbol === s.symbol;
							return (
								<button
									key={s.symbol}
									type="button"
									onClick={() =>
										setSelectedSymbol((prev) =>
											prev === s.symbol ? null : s.symbol,
										)
									}
									className={cn(
										"flex items-center gap-1.5 px-2 py-0.5 rounded border transition-all cursor-pointer font-mono shrink-0",
										isSelected
											? "bg-[#0284c7]/25 border-[#38bdf8] text-[#38bdf8] shadow"
											: "bg-(--sunken) border-(--line) text-(--f2) hover:border-[#38bdf8]/50",
									)}
								>
									<span className="w-1.5 h-1.5 rounded-full bg-[#38bdf8]" />
									<span className="font-bold">
										{s.symbol.replace("/USD", "")}
									</span>
									<span className="px-1 py-0.2 rounded bg-(--raised) text-[#38bdf8] font-bold text-[9px]">
										D{s.depth}
									</span>
									<span className="text-(--f3) text-[9px]">
										{s.tokens.join("→")}
									</span>
								</button>
							);
						})}
						{selectedSymbol && (
							<button
								type="button"
								onClick={() => setSelectedSymbol(null)}
								className="text-[9px] text-(--f4) hover:text-(--f2) underline px-1 cursor-pointer shrink-0"
							>
								Clear focus
							</button>
						)}
					</div>
				)}
			</div>

			{/* Center Visualizer Canvas */}
			<div
				ref={wrapperRef}
				className="flex-1 relative bg-(--sunken) overflow-hidden"
			>
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

				{!treeData && (
					<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-xs tracking-wider">
						Loading trie...
					</div>
				)}

				<svg
					ref={svgRef}
					className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing"
					role="img"
					aria-label="Trie visualization"
				>
					<title>Trie Graph Visualization</title>
					<g transform={transform.toString()}>
						{/* Links Layer: Edges are strictly region tokens */}
						<g className="links">
							{links.map((link) => {
								const d = getPath(link.source, link.target);
								const { s, t } = getLinkPorts(link.source, link.target);
								const midX = (s.x + t.x) / 2;
								const midY = (s.y + t.y) / 2;

								const isSymbolLink = activePathPrefixes.has(
									link.target.data.prefix,
								);
								const isLinkActive = activePathIds
									? activePathIds.has(link.target.data.id)
									: selectedSymbol
										? isSymbolLink
										: true;

								const token = link.target.data.token;

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
											stroke={isSymbolLink ? "#38bdf8" : "var(--acc)"}
											strokeWidth={isSymbolLink ? 2.5 : 2}
											strokeOpacity={isSymbolLink ? 1 : 0.7}
											style={{
												transition: "all 0.3s ease-in-out",
												filter: isSymbolLink
													? "drop-shadow(0 0 3px rgba(56, 189, 248, 0.5))"
													: undefined,
											}}
										/>
										{token && (
											<g
												transform={`translate(${midX}, ${projection === "vertical" ? midY - 6 : midY - 8})`}
												className="pointer-events-none select-none font-mono"
												style={{
													transition: "transform 0.3s ease-in-out",
												}}
											>
												<rect
													x={-18}
													y={-7}
													width={36}
													height={14}
													rx={2}
													fill="var(--surface)"
													stroke={
														isSymbolLink ? "#38bdf8" : "var(--line2)"
													}
													strokeWidth={isSymbolLink ? 1.5 : 1}
													opacity={0.95}
												/>
												<text
													x={0}
													y={3.5}
													fill={
														isSymbolLink ? "#38bdf8" : "var(--acc)"
													}
													fontSize="9px"
													textAnchor="middle"
													fontWeight="600"
													className="font-mono tracking-wider"
												>
													{token}
												</text>
											</g>
										)}
									</g>
								);
							})}
						</g>

						{/* Nodes Layer: In-between nodes are empty, terminal nodes are actions */}
						<g className="nodes">
							{nodes.map((node) => {
								const nodeData = node.data;
								const isRoot = isNodeRoot(node);
								const isTerminal = isNodeTerminal(node);
								const hasChildren = !!(nodeData.children || nodeData._children);
								const isCollapsed = !!nodeData._children;
								const action = nodeData.action;

								const pos = getNodePos(node);
								const ports = getPorts(node, projection);
								const symbolsAtNode = symbolPositions.get(nodeData.prefix);
								const isSymbolOnPath = activePathPrefixes.has(
									nodeData.prefix,
								);

								const isNodeActive = activePathIds
									? activePathIds.has(nodeData.id)
									: selectedSymbol
										? isSymbolOnPath
										: true;

								return (
									// biome-ignore lint/a11y/noStaticElementInteractions: node interactivity
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
										onMouseEnter={() => setHoveredNodeId(nodeData.id)}
										onMouseLeave={() => setHoveredNodeId(null)}
										className={
											hasChildren ? "cursor-pointer" : "cursor-default"
										}
									>
										{isRoot ? (
											/* Root Node: Clean minimal junction */
											<g>
												<rect
													x={-ROOT_WIDTH / 2}
													y={-ROOT_HEIGHT / 2}
													width={ROOT_WIDTH}
													height={ROOT_HEIGHT}
													rx={3}
													fill="var(--surface)"
													stroke="var(--acc)"
													strokeWidth={1.5}
												/>
												<text
													x={0}
													y={0}
													dy="0.32em"
													textAnchor="middle"
													fill="var(--acc)"
													fontSize="9px"
													fontWeight="700"
													className="select-none pointer-events-none font-mono tracking-wider"
												>
													ROOT
												</text>
											</g>
										) : isTerminal ? (
											/* Terminal Node: Action badge */
											<g>
												<rect
													x={-TERMINAL_WIDTH / 2}
													y={-TERMINAL_HEIGHT / 2}
													width={TERMINAL_WIDTH}
													height={TERMINAL_HEIGHT}
													rx={4}
													fill={
														action === "ENTER"
															? "rgba(115, 190, 104, 0.15)"
															: action === "EXIT"
																? "rgba(240, 84, 79, 0.15)"
																: "rgba(232, 163, 61, 0.15)"
													}
													stroke={
														action === "ENTER"
															? "var(--up)"
															: action === "EXIT"
																? "var(--down)"
																: "var(--warn)"
													}
													strokeWidth={1.5}
												/>
												<text
													x={0}
													y={0}
													dy="0.32em"
													textAnchor="middle"
													fill={
														action === "ENTER"
															? "var(--up)"
															: action === "EXIT"
																? "var(--down)"
																: "var(--warn)"
													}
													fontSize="11px"
													fontWeight="700"
													className="select-none pointer-events-none font-mono tracking-wider uppercase"
												>
													{action || "NOOP"}
												</text>
											</g>
										) : (
											/* In-Between Node: Empty junction box */
											<rect
												x={-INBETWEEN_WIDTH / 2}
												y={-INBETWEEN_HEIGHT / 2}
												width={INBETWEEN_WIDTH}
												height={INBETWEEN_HEIGHT}
												rx={3}
												fill="var(--surface)"
												stroke={
													isSymbolOnPath
														? "#38bdf8"
														: isCollapsed
															? "var(--acc)"
															: "var(--line2)"
												}
												strokeWidth={
													isSymbolOnPath || isCollapsed ? 2 : 1
												}
												className="transition-colors hover:stroke-(--acc)"
											/>
										)}

										{/* Input / Parent Port Circle */}
										{ports.parentPort && (
											<circle
												cx={ports.parentPort.cx}
												cy={ports.parentPort.cy}
												r={3}
												fill="var(--surface)"
												stroke="var(--f4)"
												strokeWidth={1.5}
											/>
										)}

										{/* Output / Child Port Circle */}
										{ports.childPort && (
											<circle
												cx={ports.childPort.cx}
												cy={ports.childPort.cy}
												r={isCollapsed ? 4 : 3}
												fill={
													isCollapsed
														? "var(--acc)"
														: "var(--surface)"
												}
												stroke={
													isCollapsed
														? "var(--acc)"
														: "var(--f4)"
												}
												strokeWidth={1.5}
											/>
										)}

										{/* Floating Live Symbol Marker on Current Node */}
										{symbolsAtNode && symbolsAtNode.length > 0 && (
											<g
												transform={`translate(0, ${projection === "vertical" ? -24 : -22})`}
												className="pointer-events-none select-none font-mono"
											>
												<rect
													x={-Math.max(26, symbolsAtNode.length * 15)}
													y={-8}
													width={Math.max(52, symbolsAtNode.length * 30)}
													height={16}
													rx={4}
													fill="rgba(2, 132, 199, 0.3)"
													stroke="#38bdf8"
													strokeWidth={1.5}
												/>
												<circle
													cx={-Math.max(26, symbolsAtNode.length * 15) + 6}
													cy={0}
													r={2.5}
													fill="#38bdf8"
													className="animate-pulse"
												/>
												<text
													x={-Math.max(26, symbolsAtNode.length * 15) + 12}
													y={0}
													dy="0.32em"
													fill="#38bdf8"
													fontSize="8px"
													fontWeight="700"
													className="font-mono tracking-wider"
												>
													{symbolsAtNode
														.map((s) => s.symbol.replace("/USD", ""))
														.join(", ")}
												</text>
											</g>
										)}
									</g>
								);
							})}
						</g>
					</g>
				</svg>
			</div>
		</div>
	);
};
