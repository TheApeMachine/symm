import * as d3 from "d3";
import type { SymbolProgress, TrieNodeData } from "./types";

export const formatExploreThreshold = (threshold: number): string => {
	if (!Number.isFinite(threshold)) return "";
	return threshold.toFixed(2);
};

/** Path edge labels that are signed magnitudes (±0.023), not region ids (R01). */
export const formatTrieLinkToken = (token: string): string => {
	const signed = token.match(/^±(-?\d+(?:\.\d+)?)$/);
	if (signed) {
		const n = Number.parseFloat(signed[1]);
		return Number.isFinite(n) ? `±${Math.abs(n).toFixed(2)}` : token;
	}
	const plain = token.match(/^[-+]?\d+(?:\.\d+)?$/);
	if (plain) {
		const n = Number.parseFloat(token);
		if (!Number.isFinite(n)) return token;
		const sign = n < 0 ? "-" : token.startsWith("+") ? "+" : "";
		return `${sign}${Math.abs(n).toFixed(2)}`;
	}
	return token;
};

const thresholdOpacity = (threshold: number): number => {
	const span = 0.95 - 0.05;
	const towardPrune = (Math.min(0.95, Math.max(0.05, threshold)) - 0.05) / span;
	return 1 - towardPrune * 0.72;
};

export type TreeNode = d3.HierarchyPointNode<TrieNodeData>;
export type TreeLink = d3.HierarchyPointLink<TrieNodeData>;

const INBETWEEN_HEIGHT = 12;
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

const getNodePos = (
	node: TreeNode,
	projection: "horizontal" | "vertical" | "radial",
) => {
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

const getLinkPorts = (
	source: TreeNode,
	target: TreeNode,
	projection: "horizontal" | "vertical" | "radial",
) => {
	const sPos = getNodePos(source, projection);
	const tPos = getNodePos(target, projection);
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

const getPath = (
	source: TreeNode,
	target: TreeNode,
	projection: "horizontal" | "vertical" | "radial",
): string | undefined => {
	if (projection === "radial") {
		return (
			d3
				.linkRadial<TreeLink, TreeNode>()
				.angle((d: TreeNode) => d.x)
				.radius((d: TreeNode) => d.y)({ source, target }) ?? undefined
		);
	}

	const { s, t } = getLinkPorts(source, target, projection);

	if (projection === "vertical") {
		return (
			d3
				// biome-ignore lint/suspicious/noExplicitAny: d3 vertical link typing
				.linkVertical<any, { x: number; y: number }>()
				.x((d) => d.x)
				.y((d) => d.y)({ source: s, target: t }) ?? undefined
		);
	}

	return (
		d3
			// biome-ignore lint/suspicious/noExplicitAny: d3 horizontal link typing
			.linkHorizontal<any, { x: number; y: number }>()
			.x((d) => d.x)
			.y((d) => d.y)({ source: s, target: t }) ?? undefined
	);
};

const linkKey = (link: TreeLink) =>
	`${link.source.data.id}-${link.target.data.id}`;

export type RadixTreeSceneInput = {
	zoomRoot: SVGGElement;
	nodes: TreeNode[];
	links: TreeLink[];
	projection: "horizontal" | "vertical" | "radial";
	rejectedPrefixes: Set<string>;
	traversedPrefixes: Set<string>;
	symbolPositions: Map<string, SymbolProgress[]>;
	activePathIds: Set<string> | null;
	selectedSymbol: string | null;
	onNodeClick: (node: TrieNodeData) => void;
	onNodeHover: (nodeId: string | null) => void;
};

const ensureLayers = (zoomRoot: SVGGElement) => {
	const root = d3.select(zoomRoot);
	if (root.select("g.links-layer").empty()) {
		root.append("g").attr("class", "links-layer");
	}
	if (root.select("g.nodes-layer").empty()) {
		root.append("g").attr("class", "nodes-layer");
	}
};

const branchFadeFor = (threshold: number | undefined) =>
	typeof threshold === "number" ? thresholdOpacity(threshold) : 1;

const linkActivityOpacity = (
	link: TreeLink,
	activePathIds: Set<string> | null,
	selectedSymbol: string | null,
	rejectedPrefixes: Set<string>,
	traversedPrefixes: Set<string>,
) => {
	const linkPrefix = grammarPrefix(link.target.data);
	const isRejected =
		rejectedPrefixes.has(linkPrefix) && !traversedPrefixes.has(linkPrefix);
	const isSymbolLink = traversedPrefixes.has(linkPrefix);
	const isLinkActive = activePathIds
		? activePathIds.has(link.target.data.id)
		: selectedSymbol
			? isSymbolLink || isRejected
			: true;
	return (isLinkActive ? 1 : 0.15) * branchFadeFor(link.target.data.threshold);
};

const nodeActivityOpacity = (
	node: TreeNode,
	activePathIds: Set<string> | null,
	selectedSymbol: string | null,
	traversedPrefixes: Set<string>,
) => {
	const isSymbolOnPath = traversedPrefixes.has(grammarPrefix(node.data));
	const isNodeActive = activePathIds
		? activePathIds.has(node.data.id)
		: selectedSymbol
			? isSymbolOnPath
			: true;
	return (isNodeActive ? 1 : 0.15) * branchFadeFor(node.data.threshold);
};

const linkStroke = (
	isRejected: boolean,
	isSymbolLink: boolean,
	branchThreshold: number | undefined,
) => {
	if (isRejected) return "var(--warn)";
	if (isSymbolLink) return "#38bdf8";
	if (typeof branchThreshold === "number") {
		if (branchThreshold < 0.35) return "var(--up)";
		if (branchThreshold > 0.65) return "var(--down)";
	}
	return "var(--line2)";
};

const paintLinks = (input: RadixTreeSceneInput, paintActivity: boolean) => {
	const {
		zoomRoot,
		links,
		projection,
		rejectedPrefixes,
		traversedPrefixes,
		activePathIds,
		selectedSymbol,
	} = input;

	const layer = d3.select(zoomRoot).select<SVGGElement>("g.links-layer");
	const linkGroups = layer
		.selectAll<SVGGElement, TreeLink>("g.trie-link")
		.data(links, linkKey);

	linkGroups.exit().remove();

	const linkEnter = linkGroups
		.enter()
		.append("g")
		.attr("class", "trie-link");

	linkEnter.append("path").attr("class", "trie-link-path");
	const label = linkEnter.append("g").attr("class", "trie-link-label");
	label.append("rect");
	label.append("text");

	const linkMerge = linkEnter.merge(linkGroups);

	linkMerge.each(function (link) {
		const group = d3.select(this);
		const linkPrefix = grammarPrefix(link.target.data);
		const isRejected =
			rejectedPrefixes.has(linkPrefix) && !traversedPrefixes.has(linkPrefix);
		const isSymbolLink = traversedPrefixes.has(linkPrefix);
		const branchThreshold = link.target.data.threshold;

		if (paintActivity) {
			group.style(
				"opacity",
				String(
					linkActivityOpacity(
						link,
						activePathIds,
						selectedSymbol,
						rejectedPrefixes,
						traversedPrefixes,
					),
				),
			);
		}

		const pathD = getPath(link.source, link.target, projection);
		group
			.select("path.trie-link-path")
			.attr("d", pathD ?? "")
			.attr("fill", "none")
			.attr(
				"stroke",
				linkStroke(isRejected, isSymbolLink, branchThreshold),
			)
			.attr("stroke-width", isRejected || isSymbolLink ? 2.5 : 1.2)
			.attr("stroke-opacity", isRejected || isSymbolLink ? 1 : 0.6)
			.attr("stroke-dasharray", isRejected || isSymbolLink ? null : "4 2")
			.style(
				"filter",
				isRejected
					? "drop-shadow(0 0 3px rgba(230, 160, 60, 0.55))"
					: isSymbolLink
						? "drop-shadow(0 0 3px rgba(56, 189, 248, 0.5))"
						: "none",
			);

		const token = link.target.data.token;
		const labelG = group.select<SVGGElement>("g.trie-link-label");
		if (!token) {
			labelG.style("display", "none");
			return;
		}

		const { s, t } = getLinkPorts(link.source, link.target, projection);
		const midX = (s.x + t.x) / 2;
		const midY = (s.y + t.y) / 2;
		const labelY = projection === "vertical" ? midY - 6 : midY - 8;

		labelG.style("display", null).attr(
			"transform",
			`translate(${midX}, ${labelY})`,
		);
		labelG
			.select("rect")
			.attr("x", -18)
			.attr("y", -7)
			.attr("width", 36)
			.attr("height", 14)
			.attr("rx", 2)
			.attr("fill", "var(--surface)")
			.attr(
				"stroke",
				isRejected
					? "var(--warn)"
					: isSymbolLink
						? "#38bdf8"
						: "var(--line2)",
			)
			.attr("stroke-width", isRejected || isSymbolLink ? 1.5 : 1)
			.attr("opacity", 0.95);

		labelG
			.select("text")
			.attr("x", 0)
			.attr("y", 3.5)
			.attr("fill", isRejected ? "var(--warn)" : isSymbolLink ? "#38bdf8" : "var(--acc)")
			.attr("font-size", "9px")
			.attr("text-anchor", "middle")
			.attr("font-weight", 600)
			.text(formatTrieLinkToken(token));
	});
};

const terminalFill = (action: string | undefined) => {
	if (action === "ENTER") return "rgba(115, 190, 104, 0.15)";
	if (action === "EXIT") return "rgba(240, 84, 79, 0.15)";
	if (action === "NOOP") return "rgba(232, 163, 61, 0.2)";
	return "rgba(148, 163, 184, 0.12)";
};

const terminalStroke = (action: string | undefined) => {
	if (action === "ENTER") return "var(--up)";
	if (action === "EXIT") return "var(--down)";
	if (action === "NOOP") return "var(--warn)";
	return "var(--f4)";
};

const terminalTextFill = (action: string | undefined) => {
	if (action === "ENTER") return "var(--up)";
	if (action === "EXIT") return "var(--down)";
	if (action === "NOOP") return "var(--warn)";
	return "var(--f3)";
};

const paintNodeContents = (
	group: d3.Selection<SVGGElement, TreeNode, SVGGElement, unknown>,
	node: TreeNode,
	input: RadixTreeSceneInput,
) => {
	const { projection, symbolPositions, traversedPrefixes } = input;
	const nodeData = node.data;
	const isRoot = isNodeRoot(node);
	const isTerminal = isNodeTerminal(node);
	const hasChildren = !!(nodeData.children || nodeData._children);
	const isCollapsed = !!nodeData._children;
	const action = nodeData.action;
	const ports = getPorts(node, projection);
	const symbolsAtNode = symbolPositions.get(nodeData.prefix);
	const isSymbolOnPath = traversedPrefixes.has(grammarPrefix(nodeData));

	group.selectAll("*").remove();

	if (isRoot) {
		group
			.append("rect")
			.attr("x", -ROOT_WIDTH / 2)
			.attr("y", -ROOT_HEIGHT / 2)
			.attr("width", ROOT_WIDTH)
			.attr("height", ROOT_HEIGHT)
			.attr("rx", 3)
			.attr("fill", "var(--surface)")
			.attr("stroke", "var(--acc)")
			.attr("stroke-width", 1.5);
		group
			.append("text")
			.attr("x", 0)
			.attr("y", 0)
			.attr("dy", "0.32em")
			.attr("text-anchor", "middle")
			.attr("fill", "var(--acc)")
			.attr("font-size", "9px")
			.attr("font-weight", 700)
			.text("ROOT");
	} else if (isTerminal) {
		if (typeof nodeData.threshold === "number") {
			group.append("title").text(
				`threshold ${formatExploreThreshold(nodeData.threshold)}`,
			);
		}
		const hasSymbol = !!(symbolsAtNode && symbolsAtNode.length > 0);
		group
			.append("rect")
			.attr("x", -TERMINAL_WIDTH / 2)
			.attr("y", -TERMINAL_HEIGHT / 2)
			.attr("width", TERMINAL_WIDTH)
			.attr("height", TERMINAL_HEIGHT)
			.attr("rx", 4)
			.attr("fill", terminalFill(action))
			.attr("stroke", terminalStroke(action))
			.attr("stroke-width", hasSymbol ? 2.5 : 1.2)
			.attr("stroke-dasharray", hasSymbol ? null : "3 2")
			.attr("opacity", hasSymbol ? 1 : 0.75);
		group
			.append("text")
			.attr("x", 0)
			.attr("y", 0)
			.attr("dy", "0.32em")
			.attr("text-anchor", "middle")
			.attr("fill", terminalTextFill(action))
			.attr("font-size", "11px")
			.attr("font-weight", 700)
			.text(action ?? "");
		if (typeof nodeData.threshold === "number") {
			group
				.append("text")
				.attr("x", 0)
				.attr("y", TERMINAL_HEIGHT / 2 + 9)
				.attr("text-anchor", "middle")
				.attr("fill", "var(--f2)")
				.attr("font-size", "8px")
				.text(formatExploreThreshold(nodeData.threshold));
		}
	} else {
		if (typeof nodeData.threshold === "number") {
			group.append("title").text(
				`threshold ${formatExploreThreshold(nodeData.threshold)}`,
			);
		}
		group
			.append("rect")
			.attr("x", -INBETWEEN_WIDTH / 2)
			.attr("y", -INBETWEEN_HEIGHT / 2)
			.attr("width", INBETWEEN_WIDTH)
			.attr("height", INBETWEEN_HEIGHT)
			.attr("rx", 3)
			.attr("fill", "var(--surface)")
			.attr("stroke", isSymbolOnPath ? "#38bdf8" : isCollapsed ? "var(--acc)" : "var(--line2)")
			.attr("stroke-width", isSymbolOnPath || isCollapsed ? 2 : 1);
	}

	if (ports.parentPort) {
		group
			.append("circle")
			.attr("cx", ports.parentPort.cx)
			.attr("cy", ports.parentPort.cy)
			.attr("r", 3)
			.attr("fill", "var(--surface)")
			.attr("stroke", "var(--f4)")
			.attr("stroke-width", 1.5);
	}

	if (ports.childPort) {
		group
			.append("circle")
			.attr("cx", ports.childPort.cx)
			.attr("cy", ports.childPort.cy)
			.attr("r", isCollapsed ? 4 : 3)
			.attr("fill", isCollapsed ? "var(--acc)" : "var(--surface)")
			.attr("stroke", isCollapsed ? "var(--acc)" : "var(--f4)")
			.attr("stroke-width", 1.5);
	}

	group.attr(
		"cursor",
		hasChildren ? "pointer" : "default",
	);
};

const paintNodes = (input: RadixTreeSceneInput, paintActivity: boolean) => {
	const {
		zoomRoot,
		nodes,
		projection,
		traversedPrefixes,
		activePathIds,
		selectedSymbol,
		onNodeClick,
		onNodeHover,
	} = input;

	const layer = d3.select(zoomRoot).select<SVGGElement>("g.nodes-layer");
	const nodeGroups = layer
		.selectAll<SVGGElement, TreeNode>("g.trie-node")
		.data(nodes, (node) => node.data.id);

	nodeGroups.exit().remove();

	const nodeEnter = nodeGroups.enter().append("g").attr("class", "trie-node");

	const nodeMerge = nodeEnter.merge(nodeGroups);

	nodeMerge
		.on("click", (event, node) => {
			event.stopPropagation();
			const hasChildren = !!(node.data.children || node.data._children);
			if (hasChildren) {
				onNodeClick(node.data);
			}
		})
		.on("mouseenter", (_, node) => {
			onNodeHover(node.data.id);
		})
		.on("mouseleave", () => {
			onNodeHover(null);
		});

	nodeMerge.each(function (node) {
		const group = d3.select(this);
		const pos = getNodePos(node, projection);
		group.attr("transform", `translate(${pos.x}, ${pos.y})`);

		if (paintActivity) {
			group.style(
				"opacity",
				String(
					nodeActivityOpacity(
						node,
						activePathIds,
						selectedSymbol,
						traversedPrefixes,
					),
				),
			);
		}

		paintNodeContents(
			group as unknown as d3.Selection<
				SVGGElement,
				TreeNode,
				SVGGElement,
				unknown
			>,
			node,
			input,
		);
	});
};

export type RadixTreeHighlightInput = Pick<
	RadixTreeSceneInput,
	| "zoomRoot"
	| "links"
	| "nodes"
	| "activePathIds"
	| "selectedSymbol"
	| "rejectedPrefixes"
	| "traversedPrefixes"
>;

export const updateRadixTreeHighlights = (input: RadixTreeHighlightInput) => {
	const {
		zoomRoot,
		links,
		nodes,
		activePathIds,
		selectedSymbol,
		rejectedPrefixes,
		traversedPrefixes,
	} = input;
	if (!zoomRoot) return;

	const root = d3.select(zoomRoot);
	root
		.select<SVGGElement>("g.links-layer")
		.selectAll<SVGGElement, TreeLink>("g.trie-link")
		.data(links, linkKey)
		.style("opacity", (link) =>
			String(
				linkActivityOpacity(
					link,
					activePathIds,
					selectedSymbol,
					rejectedPrefixes,
					traversedPrefixes,
				),
			),
		);

	root
		.select<SVGGElement>("g.nodes-layer")
		.selectAll<SVGGElement, TreeNode>("g.trie-node")
		.data(nodes, (node) => node.data.id)
		.style("opacity", (node) =>
			String(
				nodeActivityOpacity(
					node,
					activePathIds,
					selectedSymbol,
					traversedPrefixes,
				),
			),
		);
};

export const paintRadixTreeStructure = (
	input: RadixTreeSceneInput,
) => {
	if (!input.zoomRoot) return;
	ensureLayers(input.zoomRoot);
	paintLinks(input, false);
	paintNodes(input, false);
};

export const paintRadixTreeScene = (input: RadixTreeSceneInput) => {
	if (!input.zoomRoot) return;
	ensureLayers(input.zoomRoot);
	paintLinks(input, true);
	paintNodes(input, true);
};
