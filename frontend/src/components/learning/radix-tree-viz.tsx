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

const grammarPrefix = (node: TrieNodeData) =>
	node.action
		? node.prefix.slice(0, node.prefix.lastIndexOf("/"))
		: node.prefix;

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

/*
grammarKey drops the store location so the tree is the token path. Train
writes policy/current/<path>/<action>.bin.
*/
export function grammarKey(key: string): string {
	const current = "policy/current/";
	if (key.startsWith(current)) return key.slice(current.length);
	return key;
}

export function buildTreeFromPrefixes(
	prefixes: string[],
): TrieNodeData | null {
	if (!prefixes || prefixes.length === 0) return null;

	const root: TrieNodeData = {
		id: "root",
		prefix: "root",
		token: "",
		children: [],
	};

	let nextId = 1;

	for (const raw of prefixes) {
		const parts = grammarKey(raw).split("/").filter(Boolean);
		if (parts.length === 0) continue;

		const lastPart = parts[parts.length - 1];
		const isAction = lastPart.endsWith(".bin");
		const action = isAction
			? lastPart.replace(/\.bin$/i, "").toUpperCase()
			: null;
		const regionTokens = isAction ? parts.slice(0, -1) : parts;
		if (regionTokens.length === 0) continue;

		let current = root;
		let prefix = "";

		for (const token of regionTokens) {
			prefix = prefix ? `${prefix}/${token}` : token;
			if (!current.children) current.children = [];
			let child = current.children.find((c) => c.token === token && !c.action);
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
			if (!current.children) current.children = [];
			const leaf = current.children.find((child) => child.action === action);
			if (!leaf) {
				current.children.push({
					id: String(nextId++),
					prefix: `${prefix}/${action.toLowerCase()}.bin`,
					token: "",
					action,
					children: [],
				});
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
			paths.push(`${nextPath.join("/")}/${node.action.toLowerCase()}.bin`);
		}
		const children = [...(node.children || []), ...(node._children || [])];
		for (const child of children) {
			walk(child, nextPath);
		}
	};

	walk(data, []);
	return paths;
}

/*
keyMatchesPrefix checks if a trie key (e.g. "R01-R02/R03-R04/R05-R06/enter.bin")
matches or extends the token prefix sequence of a symbol (e.g. ["R01-R02", "R03-R04"]).
*/
export function keyMatchesPrefix(key: string, tokens: string[]): boolean {
	if (!tokens || tokens.length === 0) return false;
	const parts = grammarKey(key).split("/").filter(Boolean);
	const lastPart = parts[parts.length - 1];
	const isAction = lastPart?.endsWith(".bin");
	const keyTokens = isAction ? parts.slice(0, -1) : parts;
	if (keyTokens.length < tokens.length) {
		return false;
	}

	for (let i = 0; i < tokens.length; i++) {
		if (keyTokens[i] !== tokens[i]) {
			return false;
		}
	}
	return true;
}

/*
keyMatchesSymbol checks if a trie key matches a symbol either through its
active tokens prefix or through its matched keys list from Step telemetry.
*/
export function keyMatchesSymbol(key: string, symbol: SymbolProgress): boolean {
	if (!symbol.tokens || symbol.tokens.length === 0) return false;

	if (symbol.matched && symbol.matched.length > 0) {
		const keyClean = grammarKey(key).replace(/\.(bin)$/i, "");
		for (const m of symbol.matched) {
			const mClean = grammarKey(m).replace(/\.(bin)$/i, "");
			if (
				keyClean === mClean ||
				keyClean.startsWith(`${mClean}/`) ||
				mClean.startsWith(`${keyClean}/`)
			) {
				return true;
			}
		}
	}

	return keyMatchesPrefix(key, symbol.tokens);
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
	const [showAllBranches, setShowAllBranches] = useState(true);
	type BranchFilter = "matching" | "enter" | "exit" | "all";
	const [branchFilter, setBranchFilter] = useState<BranchFilter>("all");

	const activeSymbolList = useMemo(() => {
		if (!symbols) return [];
		return Object.values(symbols).filter(
			(s): s is SymbolProgress => !!s && !!s.path && s.depth > 0,
		);
	}, [symbols]);

	const targetSymbols = useMemo(() => {
		if (selectedSymbol) {
			return activeSymbolList.filter((s) => s.symbol === selectedSymbol);
		}
		return activeSymbolList;
	}, [activeSymbolList, selectedSymbol]);

	const allKeys = useMemo(() => {
		if (keys && keys.length > 0) return keys;
		if (data) {
			const extracted = extractPrefixes(data);
			if (extracted.length > 0) return extracted;
		}
		return [];
	}, [keys, data]);

	const actionCounts = useMemo(() => {
		let enter = 0;
		let exit = 0;
		for (const key of allKeys) {
			if (key.endsWith("/enter.bin")) enter++;
			else if (key.endsWith("/exit.bin")) exit++;
		}
		return { enter, exit, total: allKeys.length };
	}, [allKeys]);

	const matchingKeys = useMemo(() => {
		if (allKeys.length === 0 || targetSymbols.length === 0) return [];
		return allKeys.filter((key) =>
			targetSymbols.some((s) => keyMatchesSymbol(key, s)),
		);
	}, [allKeys, targetSymbols]);

	const filteredKeys = useMemo(() => {
		if (allKeys.length === 0) return [];
		if (showAllBranches) return allKeys;
		switch (branchFilter) {
			case "all":
				return allKeys;
			case "enter":
				return allKeys.filter((k) => k.endsWith("/enter.bin"));
			case "exit":
				return allKeys.filter((k) => k.endsWith("/exit.bin"));
			case "matching":
				return matchingKeys;
			default: {
				const unreachable: never = branchFilter;
				return unreachable;
			}
		}
	}, [allKeys, branchFilter, showAllBranches, matchingKeys]);

	const rejectedPrefixes = useMemo(() => {
		const set = new Set<string>();
		for (const item of targetSymbols) {
			const base = item.tokens.filter(Boolean).join("/");
			for (const token of item.rejected ?? []) {
				set.add(base ? `${base}/${token}` : token);
			}
		}
		return set;
	}, [targetSymbols]);

	// Set of prefix paths ACTUALLY reached by at least one active symbol
	const traversedPrefixes = useMemo(() => {
		const set = new Set<string>();
		for (const item of targetSymbols) {
			if (!item.tokens) continue;
			let prefix = "";
			for (const tok of item.tokens) {
				prefix = prefix ? `${prefix}/${tok}` : tok;
				set.add(prefix);
			}
		}
		return set;
	}, [targetSymbols]);

	// Set of all prefix paths present in current displayed tree
	const treePrefixes = useMemo(() => {
		const set = new Set<string>();
		for (const key of filteredKeys) {
			const parts = key.split("/").filter(Boolean);
			const isAction = parts[parts.length - 1]?.endsWith(".bin");
			const regionTokens = isAction ? parts.slice(0, -1) : parts;
			let prefix = "";
			for (const tok of regionTokens) {
				prefix = prefix ? `${prefix}/${tok}` : tok;
				set.add(prefix);
			}
		}
		return set;
	}, [filteredKeys]);

	// Map of exact prefix to symbols currently located at that node
	const symbolPositions = useMemo(() => {
		const map = new Map<string, SymbolProgress[]>();
		for (const item of targetSymbols) {
			let bestPrefix = "";
			let current = "";
			for (const tok of item.tokens) {
				current = current ? `${current}/${tok}` : tok;
				if (treePrefixes.has(current)) {
					bestPrefix = current;
				}
			}
			const targetPrefix = bestPrefix || item.path;
			const existing = map.get(targetPrefix) || [];
			existing.push(item);
			map.set(targetPrefix, existing);
		}
		return map;
	}, [targetSymbols, treePrefixes]);

	const treeData = useMemo(() => {
		if (filteredKeys && filteredKeys.length > 0) {
			return buildTreeFromPrefixes(filteredKeys);
		}
		if (showAllBranches && allKeys.length > 0) {
			return buildTreeFromPrefixes(allKeys);
		}
		return null;
	}, [filteredKeys, allKeys, showAllBranches]);

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
			.extent([
				[0, 0],
				[dimensions.width, dimensions.height],
			])
			.scaleExtent([0.1, 4])
			.on("zoom", (zoomEvent) => {
				setTransform(zoomEvent.transform);
			});

		zoomBehavior.current = zoom;
		svg.call(zoom);

		let initialTransform = d3.zoomIdentity
			.translate(80, dimensions.height / 2)
			.scale(0.85);
		if (projection === "radial") {
			initialTransform = d3.zoomIdentity
				.translate(dimensions.width / 2, dimensions.height / 2)
				.scale(0.8);
		}
		if (projection === "vertical") {
			initialTransform = d3.zoomIdentity
				.translate(dimensions.width / 2, 60)
				.scale(0.85);
		}
		svg.call(zoom.transform, initialTransform);
	}, [dimensions.width, dimensions.height, projection]);

	const handleNodeClick = (nodeData: TrieNodeData) => {
		setCollapsedIds((previousIds) => {
			const nextIds = new Set(previousIds);
			if (nextIds.has(nodeData.id)) {
				nextIds.delete(nodeData.id);
				return nextIds;
			}
			nextIds.add(nodeData.id);
			return nextIds;
		});
	};

	const { nodes, links } = useMemo(() => {
		if (!treeData) return { nodes: [], links: [] };

		// Clone and apply collapsing
		const prepareHierarchy = (nodeItem: TrieNodeData): TrieNodeData => {
			const isCollapsed = collapsedIds.has(nodeItem.id);
			const copy: TrieNodeData = { ...nodeItem };
			if (!nodeItem.children || nodeItem.children.length === 0) {
				return copy;
			}
			if (isCollapsed) {
				copy._children = nodeItem.children;
				copy.children = undefined;
				return copy;
			}
			copy.children = nodeItem.children.map(prepareHierarchy);
			return copy;
		};

		const displayTree = prepareHierarchy(treeData);
		const root = d3.hierarchy<TrieNodeData>(displayTree);
		const treeLayout = d3.tree<TrieNodeData>();

		if (projection === "radial") {
			let maxDepth = 0;
			root.each((depthNode) => {
				if (depthNode.depth > maxDepth) maxDepth = depthNode.depth;
			});
			const radius = Math.max(300, maxDepth * 160);
			treeLayout.size([2 * Math.PI, radius]);
		}
		if (projection === "vertical") {
			treeLayout.nodeSize([80, 110]);
		}
		if (projection === "horizontal") {
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
					<div className="flex gap-3 items-center">
						<span className="text-(--acc) border-(--acc) border-b py-1 text-[11px] font-bold tracking-wider">
							TRIE
						</span>
						<span className="text-(--f3) text-[11px]">
							{nodes.length} nodes
						</span>
						{activeSymbolList.length > 0 && !showAllBranches && (
							<span className="text-(--info) text-[11px] font-medium flex items-center gap-1.5">
								<span className="w-1.5 h-1.5 rounded-full bg-(--info) animate-pulse" />
								{filteredKeys.length} matching{" "}
								{filteredKeys.length === 1 ? "branch" : "branches"} (
								{targetSymbols.length} active{" "}
								{targetSymbols.length === 1 ? "symbol" : "symbols"})
							</span>
						)}
						{showAllBranches && (
							<span className="text-(--warn) text-[11px] font-medium flex items-center gap-1.5">
								Showing all {allKeys.length} branches
							</span>
						)}
					</div>

					<div className="flex items-center gap-2">
						<div className="flex items-center gap-1 bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px] font-mono">
							<button
								type="button"
								onClick={() => {
									setBranchFilter("matching");
									setShowAllBranches(false);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors cursor-pointer",
									branchFilter === "matching" && !showAllBranches
										? "bg-(--raised) text-(--info) font-bold"
										: "text-(--f3) hover:text-(--f1)",
								)}
							>
								Matching ({matchingKeys.length})
							</button>
							<button
								type="button"
								onClick={() => {
									setBranchFilter("enter");
									setShowAllBranches(false);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors cursor-pointer",
									branchFilter === "enter"
										? "bg-(--raised) text-(--up) font-bold"
										: "text-(--up)/70 hover:text-(--up)",
								)}
							>
								ENTER ({actionCounts.enter})
							</button>
							<button
								type="button"
								onClick={() => {
									setBranchFilter("exit");
									setShowAllBranches(false);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors cursor-pointer",
									branchFilter === "exit"
										? "bg-(--raised) text-(--down) font-bold"
										: "text-(--down)/70 hover:text-(--down)",
								)}
							>
								EXIT ({actionCounts.exit})
							</button>
							<button
								type="button"
								onClick={() => {
									setBranchFilter("all");
									setShowAllBranches(true);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors cursor-pointer",
									branchFilter === "all" || showAllBranches
										? "bg-(--raised) text-(--acc) font-bold"
										: "text-(--f3) hover:text-(--f1)",
								)}
							>
								All ({allKeys.length})
							</button>
						</div>

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
											? "bg-(--info)/25 border-(--info) text-(--info) shadow"
											: "bg-(--sunken) border-(--line) text-(--f2) hover:border-(--info)/50",
									)}
								>
									<span className="w-1.5 h-1.5 rounded-full bg-(--info)" />
									<span className="font-bold">
										{s.symbol.replace("/USD", "")}
									</span>
									<span className="px-1 py-0.2 rounded bg-(--raised) text-(--info) font-bold text-[9px]">
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
					<div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-(--f3) p-4 text-center select-none pointer-events-auto">
						<div className="w-12 h-12 rounded-full border border-(--line) flex items-center justify-center text-(--f4) mb-1">
							<svg
								width="22"
								height="22"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								strokeWidth="1.5"
							>
								<title>Empty Trie</title>
								<circle cx="6" cy="6" r="3" />
								<circle cx="6" cy="18" r="3" />
								<circle cx="18" cy="9" r="3" />
								<path d="M6 9v6" />
								<path d="M6 9a9 9 0 0 1 9-3h3" />
							</svg>
						</div>
						<span className="text-xs font-semibold text-(--f2)">
							{targetSymbols.length > 0
								? "No Stored Branches Match Current Live Prefixes"
								: "No Active Symbol Sequences"}
						</span>
						<span className="text-[11px] text-(--f4) max-w-sm">
							{targetSymbols.length > 0
								? `None of the ${targetSymbols.length} active symbols match the 4-depth policy at their current prefix.`
								: "No symbol currently has a built-up sequence of region IDs. Branches matching live token paths will appear as Step advances."}
						</span>
						{allKeys.length > 0 && (
							<div className="flex items-center gap-2 mt-2">
								<button
									type="button"
									onClick={() => {
										setBranchFilter("all");
										setShowAllBranches(true);
									}}
									className="px-3 py-1 text-xs rounded bg-(--surface) border border-(--line) text-(--acc) hover:bg-(--raised) cursor-pointer font-mono"
								>
									Show all {allKeys.length} stored branches
								</button>
							</div>
						)}
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

								const linkPrefix = grammarPrefix(link.target.data);
								const isRejected =
									rejectedPrefixes.has(linkPrefix) &&
									!traversedPrefixes.has(linkPrefix);
								const isSymbolLink = traversedPrefixes.has(linkPrefix);
								const isLinkActive = activePathIds
									? activePathIds.has(link.target.data.id)
									: selectedSymbol
										? isSymbolLink || isRejected
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
											stroke={
												isRejected
													? "var(--warn)"
													: isSymbolLink
														? "#38bdf8"
														: "var(--line2)"
											}
											strokeWidth={isRejected || isSymbolLink ? 2.5 : 1.2}
											strokeOpacity={isRejected || isSymbolLink ? 1 : 0.6}
											strokeDasharray={
												isRejected || isSymbolLink ? undefined : "4 2"
											}
											style={{
												transition: "all 0.3s ease-in-out",
												filter: isRejected
													? "drop-shadow(0 0 3px rgba(230, 160, 60, 0.55))"
													: isSymbolLink
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
														isRejected
															? "var(--warn)"
															: isSymbolLink
																? "#38bdf8"
																: "var(--line2)"
													}
													strokeWidth={isRejected || isSymbolLink ? 1.5 : 1}
													opacity={0.95}
												/>
												<text
													x={0}
													y={3.5}
													fill={
														isRejected
															? "var(--warn)"
															: isSymbolLink
																? "#38bdf8"
																: "var(--acc)"
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
								const isSymbolOnPath = traversedPrefixes.has(
									grammarPrefix(nodeData),
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
																: action === "NOOP"
																	? "rgba(232, 163, 61, 0.2)"
																	: "rgba(148, 163, 184, 0.12)"
													}
													stroke={
														action === "ENTER"
															? "var(--up)"
															: action === "EXIT"
																? "var(--down)"
																: action === "NOOP"
																	? "var(--warn)"
																	: "var(--f4)"
													}
													strokeWidth={
														symbolsAtNode && symbolsAtNode.length > 0
															? 2.5
															: 1.2
													}
													strokeDasharray={
														symbolsAtNode && symbolsAtNode.length > 0
															? undefined
															: "3 2"
													}
													opacity={
														symbolsAtNode && symbolsAtNode.length > 0 ? 1 : 0.75
													}
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
																: action === "NOOP"
																	? "var(--warn)"
																	: "var(--f3)"
													}
													fontSize="11px"
													fontWeight="700"
													className="select-none pointer-events-none font-mono tracking-wider uppercase"
												>
													{action}
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
												strokeWidth={isSymbolOnPath || isCollapsed ? 2 : 1}
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
												fill={isCollapsed ? "var(--acc)" : "var(--surface)"}
												stroke={isCollapsed ? "var(--acc)" : "var(--f4)"}
												strokeWidth={1.5}
											/>
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
