import * as d3 from "d3";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "../ui/flex";
import { EXPLORE_BASE_THRESHOLD, explorePathKept } from "./explore-path";
import {
	formatTrieLinkToken,
	paintRadixTreeStructure,
	type TreeNode,
	updateRadixTreeHighlights,
} from "./radix-tree-paint";
import type { FeasibleAction, SymbolProgress, TrieNodeData } from "./types";

interface RadixTreeVizProps {
	data?: TrieNodeData | null;
	keys?: string[] | null;
	thresholds?: Record<string, number> | null;
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

/** Above this count the trie is summarized instead of fully laid out in the scene. */
export const MAX_TRIE_KEYS = 1_500;

/*
grammarKey drops the store location so the tree is the token path.
Confirmed paths are policy/current/<path>/<action>.bin. Enter paths end
precursor A→B; exit paths are a separate holding B→C walk from root.
Virtual explore paths are model/paths/<path>/{enter,hold,exit}.bin.
Paper policy paths are model/policy/<path>/{enter,hold,exit}.bin.
Legacy explored paths are policy/explored/<action>/<path>.bin.
*/
export function grammarKey(key: string): string {
	for (const prefix of ["model/paths"]) {
		if (key.startsWith(prefix)) {
			return key.slice(prefix.length);
		}
	}

	for (const prefix of ["policy/current/", "model/current/"]) {
		if (key.startsWith(prefix)) {
			return key.slice(prefix.length);
		}
	}

	const explored = "policy/explored/";
	if (key.startsWith(explored)) {
		const rest = key.slice(explored.length);
		const slash = rest.indexOf("/");
		if (slash <= 0) return rest;
		const role = rest.slice(0, slash);
		const path = rest.slice(slash + 1).replace(/\.bin$/i, "");
		return path === "" ? rest : `${path}/${role}.bin`;
	}

	return key;
}

/*
A branch starts at 0.50. Profit walks it toward 0.05, where it is solid.
Loss walks it toward 0.95, where it fades.
*/
export const thresholdOpacity = (threshold: number): number => {
	const span = 0.95 - 0.05;
	const towardPrune = (Math.min(0.95, Math.max(0.05, threshold)) - 0.05) / span;
	return 1 - towardPrune * 0.72;
};

const exploreLeafThreshold = (
	rawKey: string,
	threshold: number | undefined,
): number | undefined => {
	if (typeof threshold === "number" && Number.isFinite(threshold)) {
		return threshold;
	}
	if (rawKey.startsWith("model/paths")) {
		return EXPLORE_BASE_THRESHOLD;
	}
	return undefined;
};

const shadeThresholds = (node: TrieNodeData): number | null => {
	const children = node.children ?? [];
	const fromChildren = children
		.map(shadeThresholds)
		.filter((value): value is number => value !== null);

	if (node.action) {
		return typeof node.threshold === "number" ? node.threshold : null;
	}

	if (fromChildren.length === 0) return null;

	node.threshold =
		fromChildren.reduce((sum, value) => sum + value, 0) / fromChildren.length;
	return node.threshold;
};

export function buildTreeFromPrefixes(
	prefixes: string[],
	thresholds?: Record<string, number> | null,
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
			const resolved = exploreLeafThreshold(raw, thresholds?.[raw]);
			if (!leaf) {
				current.children.push({
					id: String(nextId++),
					prefix: `${prefix}/${action.toLowerCase()}.bin`,
					token: "",
					action,
					threshold: resolved,
					children: [],
				});
			} else if (resolved !== undefined) {
				leaf.threshold = resolved;
			}
		}
	}

	shadeThresholds(root);
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
	thresholds,
	symbols,
	projection: externalProjection,
	onProjectionChange,
	className,
}) => {
	const svgRef = useRef<SVGSVGElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const zoomLayerRef = useRef<SVGGElement>(null);
	const gridRef = useRef<HTMLDivElement>(null);
	const zoomBehavior = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(
		null,
	);
	const transformRef = useRef<d3.ZoomTransform>(d3.zoomIdentity);
	const zoomProjectionRef = useRef<typeof projection | null>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
	const [internalProjection, setInternalProjection] = useState<
		"horizontal" | "vertical" | "radial"
	>("horizontal");

	const projection = externalProjection ?? internalProjection;
	const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);
	const [showAllBranches, setShowAllBranches] = useState(false);
	type BranchFilter = "matching" | "enter" | "hold" | "exit" | "all";
	const [branchFilter, setBranchFilter] = useState<BranchFilter>("matching");

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

	const { allKeys, keysTruncated, totalKeyCount } = useMemo(() => {
		let resolved: string[] = [];
		if (keys && keys.length > 0) resolved = keys;
		else if (data) {
			const extracted = extractPrefixes(data);
			if (extracted.length > 0) resolved = extracted;
		}
		const kept = resolved.filter((key) => explorePathKept(key, thresholds));
		const total = kept.length;
		if (total > MAX_TRIE_KEYS) {
			return {
				allKeys: kept.slice(0, MAX_TRIE_KEYS),
				keysTruncated: true,
				totalKeyCount: total,
			};
		}
		return {
			allKeys: kept,
			keysTruncated: false,
			totalKeyCount: total,
		};
	}, [keys, data, thresholds]);

	const actionCounts = useMemo(() => {
		let enter = 0;
		let hold = 0;
		let exit = 0;
		for (const key of allKeys) {
			const grammar = grammarKey(key);
			if (grammar.endsWith("/enter.bin")) enter++;
			else if (grammar.endsWith("/hold.bin")) hold++;
			else if (grammar.endsWith("/exit.bin")) exit++;
		}
		return { enter, hold, exit, total: allKeys.length };
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
				return allKeys.filter((k) => grammarKey(k).endsWith("/enter.bin"));
			case "hold":
				return allKeys.filter((k) => grammarKey(k).endsWith("/hold.bin"));
			case "exit":
				return allKeys.filter((k) => grammarKey(k).endsWith("/exit.bin"));
			case "matching":
				return matchingKeys.length > 0 ? matchingKeys : allKeys;
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
			return buildTreeFromPrefixes(filteredKeys, thresholds);
		}
		if (showAllBranches && allKeys.length > 0) {
			return buildTreeFromPrefixes(allKeys, thresholds);
		}
		return null;
	}, [filteredKeys, allKeys, showAllBranches, thresholds]);

	const [collapsedIds, setCollapsedIds] = useState<Set<string>>(new Set());

	const applyZoomTransform = useCallback((next: d3.ZoomTransform) => {
		transformRef.current = next;
		zoomLayerRef.current?.setAttribute("transform", next.toString());
		const grid = gridRef.current;
		if (grid) {
			grid.style.transform = `translate(${next.x % 24}px, ${next.y % 24}px) scale(${next.k})`;
			grid.style.transformOrigin = "0 0";
		}
	}, []);

	useEffect(() => {
		const observeTarget = wrapperRef.current;
		if (!observeTarget || typeof ResizeObserver === "undefined") return;

		const resizeObserver = new ResizeObserver((entries) => {
			if (!entries[0]) return;
			const { width, height } = entries[0].contentRect;
			if (width <= 0 || height <= 0) return;
			setDimensions((current) =>
				current.width > 0 &&
				Math.abs(current.width - width) < 4 &&
				Math.abs(current.height - height) < 4
					? current
					: { width, height },
			);
		});

		resizeObserver.observe(observeTarget);
		return () => resizeObserver.unobserve(observeTarget);
	}, []);

	const initialZoomTransform = useCallback(
		(width: number, height: number, mode: typeof projection) => {
			if (mode === "radial") {
				return d3.zoomIdentity.translate(width / 2, height / 2).scale(0.8);
			}
			if (mode === "vertical") {
				return d3.zoomIdentity.translate(width / 2, 60).scale(0.85);
			}
			return d3.zoomIdentity.translate(80, height / 2).scale(0.85);
		},
		[],
	);

	// Setup D3 Zoom — keep the user's pan/zoom unless projection changes.
	useEffect(() => {
		if (!svgRef.current || dimensions.width === 0 || dimensions.height === 0)
			return;

		const svg = d3.select(svgRef.current);
		const width = dimensions.width;
		const height = dimensions.height;

		let zoom = zoomBehavior.current;
		if (!zoom) {
			zoom = d3
				.zoom<SVGSVGElement, unknown>()
				.scaleExtent([0.1, 4])
				.on("zoom", (zoomEvent) => {
					applyZoomTransform(zoomEvent.transform);
				});
			zoomBehavior.current = zoom;
			svg.call(zoom);
		}

		zoom.extent([
			[0, 0],
			[width, height],
		]);

		const projectionChanged = zoomProjectionRef.current !== projection;
		zoomProjectionRef.current = projection;

		if (projectionChanged) {
			const initial = initialZoomTransform(width, height, projection);
			svg.call(zoom.transform, initial);
			applyZoomTransform(initial);
			return;
		}

		const current = transformRef.current;
		svg.call(zoom.transform, current);
		applyZoomTransform(current);
	}, [dimensions, projection, applyZoomTransform, initialZoomTransform]);

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

	const handleNodeClick = useCallback((nodeData: TrieNodeData) => {
		setCollapsedIds((previousIds) => {
			const nextIds = new Set(previousIds);
			if (nextIds.has(nodeData.id)) {
				nextIds.delete(nodeData.id);
				return nextIds;
			}
			nextIds.add(nodeData.id);
			return nextIds;
		});
	}, []);

	useEffect(() => {
		const root = zoomLayerRef.current;
		if (!root) return;

		if (nodes.length === 0) {
			d3.select(root).select("g.links-layer").selectAll("*").remove();
			d3.select(root).select("g.nodes-layer").selectAll("*").remove();
			return;
		}

		paintRadixTreeStructure({
			zoomRoot: root,
			nodes,
			links,
			projection,
			rejectedPrefixes,
			traversedPrefixes,
			symbolPositions,
			activePathIds: null,
			selectedSymbol,
			onNodeClick: handleNodeClick,
			onNodeHover: setHoveredNodeId,
		});
	}, [
		nodes,
		links,
		projection,
		rejectedPrefixes,
		traversedPrefixes,
		symbolPositions,
		selectedSymbol,
		handleNodeClick,
	]);

	useEffect(() => {
		const root = zoomLayerRef.current;
		if (!root || nodes.length === 0) return;

		updateRadixTreeHighlights({
			zoomRoot: root,
			nodes,
			links,
			activePathIds,
			selectedSymbol,
			rejectedPrefixes,
			traversedPrefixes,
		});
	}, [
		nodes,
		links,
		activePathIds,
		selectedSymbol,
		rejectedPrefixes,
		traversedPrefixes,
	]);

	return (
		<Flex.Column
			fullWidth
			fullHeight
			className={cn("bg-(--bg) text-(--f2) font-mono", className)}
		>
			{/* Top Bar with Projection Controls and Active Symbol Depth Ticker */}
			<Flex.Column className="border-(--line) border-b bg-(--surface) shrink-0">
				<Flex.Row align="center" justify="between" className="h-8 px-3 text-xs">
					<Flex.Row align="center" gap={3}>
						<span className="text-(--acc) border-(--acc) border-b py-1 text-[11px] font-bold tracking-wider">
							TRIE
						</span>
						<span className="text-(--f3) text-[11px]">
							{nodes.length} nodes
							{keysTruncated
								? ` · showing first ${MAX_TRIE_KEYS} of ${totalKeyCount} paths`
								: null}
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
						{thresholds && Object.keys(thresholds).length > 0 && (
							<span
								className="text-(--f4) text-[10px] font-mono"
								title="A branch starts at 0.50. Profit makes it solid. Loss fades it toward the prune."
							>
								solid 0.05 · faint 0.95
							</span>
						)}
					</Flex.Row>

					<Flex.Row align="center" gap={2}>
						<Flex.Row
							align="center"
							gap={1}
							className="bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px] font-mono"
						>
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
									setBranchFilter("hold");
									setShowAllBranches(false);
								}}
								className={cn(
									"px-2 py-0.5 rounded transition-colors cursor-pointer",
									branchFilter === "hold"
										? "bg-(--raised) text-(--info) font-bold"
										: "text-(--info)/70 hover:text-(--info)",
								)}
							>
								HOLD ({actionCounts.hold})
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
						</Flex.Row>

						<Flex.Row
							align="center"
							gap={1}
							className="bg-(--sunken) border-(--line) border p-0.5 rounded text-[11px]"
						>
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
						</Flex.Row>
					</Flex.Row>
				</Flex.Row>

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
										{s.tokens.map(formatTrieLinkToken).join("→")}
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
			</Flex.Column>

			{/* Center Visualizer Canvas */}
			<div
				ref={wrapperRef}
				className="flex-1 relative bg-(--sunken) overflow-hidden"
			>
				<div
					ref={gridRef}
					className="absolute inset-0 pointer-events-none opacity-[0.12]"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "24px 24px",
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
					<g ref={zoomLayerRef} />
				</svg>
			</div>
		</Flex.Column>
	);
};
