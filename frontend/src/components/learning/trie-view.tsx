import { AnimatePresence, motion } from "motion/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Slider } from "#/components/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "#/components/ui/toggle-group";
import { Typography } from "#/components/ui/typography";

export interface TrieNode {
	id: string;
	prefix: string;
	probability: number;
	stepProbability?: number;
	tokens?: string[];
	state?: "EVALUATED" | "POLICY CHOICE" | "ESTIMATED";
	isEnd?: boolean;
	children?: TrieNode[];
	_children?: TrieNode[];
}

export const generateDefaultTrie = (): TrieNode => ({
	id: "root",
	prefix: "ROOT",
	probability: 1.0,
	stepProbability: 1.0,
	state: "EVALUATED",
	tokens: ["<CTX>"],
	children: [
		{
			id: "wait",
			prefix: "wait",
			probability: 0.3,
			stepProbability: 0.3,
			state: "EVALUATED",
			tokens: ["wait"],
			children: [
				{
					id: "wait_100ms",
					prefix: " 100ms",
					probability: 0.2,
					stepProbability: 0.66,
					state: "EVALUATED",
					tokens: ["100", "ms"],
					isEnd: true,
				},
				{
					id: "wait_500ms",
					prefix: " 500ms",
					probability: 0.07,
					stepProbability: 0.23,
					state: "EVALUATED",
					tokens: ["500", "ms"],
					isEnd: true,
				},
				{
					id: "wait_obs",
					prefix: " (obs)",
					probability: 0.03,
					stepProbability: 0.1,
					state: "ESTIMATED",
					tokens: ["obs"],
					children: [
						{
							id: "wait_vol",
							prefix: " vol",
							probability: 0.02,
							stepProbability: 0.66,
							state: "ESTIMATED",
							tokens: ["vol"],
							isEnd: true,
						},
						{
							id: "wait_liq",
							prefix: " liq",
							probability: 0.01,
							stepProbability: 0.33,
							state: "ESTIMATED",
							tokens: ["liq"],
							isEnd: true,
						},
					],
				},
			],
		},
		{
			id: "enter",
			prefix: "enter",
			probability: 0.35,
			stepProbability: 0.35,
			state: "EVALUATED",
			tokens: ["enter"],
			children: [
				{
					id: "enter_1_8",
					prefix: " 1/8",
					probability: 0.2,
					stepProbability: 0.57,
					state: "EVALUATED",
					tokens: ["1", "/", "8"],
					children: [
						{
							id: "enter_ask",
							prefix: " @ask",
							probability: 0.12,
							stepProbability: 0.6,
							state: "POLICY CHOICE",
							tokens: ["ask"],
							isEnd: true,
						},
						{
							id: "enter_bid",
							prefix: " @bid",
							probability: 0.08,
							stepProbability: 0.4,
							state: "EVALUATED",
							tokens: ["bid"],
							isEnd: true,
						},
					],
				},
				{
					id: "enter_1_4",
					prefix: " 1/4",
					probability: 0.1,
					stepProbability: 0.28,
					state: "ESTIMATED",
					tokens: ["1", "/", "4"],
					children: [
						{
							id: "enter_1_4_mkt",
							prefix: " mkt",
							probability: 0.06,
							stepProbability: 0.6,
							state: "ESTIMATED",
							tokens: ["mkt"],
							isEnd: true,
						},
						{
							id: "enter_1_4_lmt",
							prefix: " lmt",
							probability: 0.04,
							stepProbability: 0.4,
							state: "ESTIMATED",
							tokens: ["lmt"],
							isEnd: true,
						},
					],
				},
				{
					id: "enter_1_2",
					prefix: " 1/2",
					probability: 0.05,
					stepProbability: 0.14,
					state: "ESTIMATED",
					tokens: ["1", "/", "2"],
					isEnd: true,
				},
			],
		},
		{
			id: "retreat",
			prefix: "retreat",
			probability: 0.15,
			stepProbability: 0.15,
			state: "ESTIMATED",
			tokens: ["retreat"],
			children: [
				{
					id: "retreat_frac",
					prefix: "_frac",
					probability: 0.1,
					stepProbability: 0.66,
					state: "ESTIMATED",
					tokens: ["frac"],
					children: [
						{
							id: "retreat_zscore",
							prefix: "_zscore",
							probability: 0.08,
							stepProbability: 0.8,
							state: "ESTIMATED",
							tokens: ["zscr"],
							isEnd: true,
						},
						{
							id: "retreat_fixed",
							prefix: "_fixed",
							probability: 0.02,
							stepProbability: 0.2,
							state: "ESTIMATED",
							tokens: ["fxd"],
							isEnd: true,
						},
					],
				},
				{
					id: "retreat_full",
					prefix: "_full",
					probability: 0.05,
					stepProbability: 0.33,
					state: "ESTIMATED",
					tokens: ["full"],
					isEnd: true,
				},
			],
		},
	],
});

interface FlatNode {
	data: TrieNode;
	x: number;
	y: number;
	depth: number;
	parentId: string | null;
}

interface FlatLink {
	source: FlatNode;
	target: FlatNode;
}

export const TrieView = () => {
	const svgRef = useRef<SVGSVGElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 500 });

	const [projection, setProjection] = useState<"horizontal" | "vertical" | "radial">("horizontal");
	const [colorMode, setColorMode] = useState<"threshold" | "gradient">("threshold");
	const [minProb, setMinProb] = useState(0.0);

	const [tree, setTree] = useState<TrieNode>(generateDefaultTrie);
	const [transform, setTransform] = useState({ x: 60, y: 180, k: 1 });
	const isDragging = useRef(false);
	const dragStart = useRef({ x: 0, y: 0 });

	const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
	const [tooltip, setTooltip] = useState<{
		data: TrieNode;
		x: number;
		y: number;
	} | null>(null);

	// Resize observer
	useEffect(() => {
		const target = wrapperRef.current;
		if (!target || typeof ResizeObserver === "undefined") return;

		const observer = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setDimensions({ width, height });
				}
			}
		});

		observer.observe(target);
		return () => observer.disconnect();
	}, []);

	// Filter tree by probability
	const filterNode = (node: TrieNode, threshold: number): TrieNode | null => {
		if (node.probability < threshold) return null;
		const clone: TrieNode = { ...node };

		if (clone.children) {
			clone.children = clone.children
				.map((c) => filterNode(c, threshold))
				.filter((c): c is TrieNode => c !== null);
		}
		if (clone._children) {
			clone._children = clone._children
				.map((c) => filterNode(c, threshold))
				.filter((c): c is TrieNode => c !== null);
		}
		return clone;
	};

	const filteredTree = useMemo(
		() => filterNode(tree, minProb) ?? tree,
		[tree, minProb],
	);

	// Compute tree layout
	const { nodes, links } = useMemo(() => {
		const flatNodes: FlatNode[] = [];
		const flatLinks: FlatLink[] = [];
		const nodeMap = new Map<string, FlatNode>();

		let leafIndex = 0;
		const assignLeafSlots = (n: TrieNode, depth: number, parent: string | null) => {
			const children = n.children ?? [];
			if (children.length === 0) {
				const fn: FlatNode = {
					data: n,
					x: 0,
					y: leafIndex++,
					depth,
					parentId: parent,
				};
				flatNodes.push(fn);
				nodeMap.set(n.id, fn);
				return fn.y;
			}

			let sumY = 0;
			for (const c of children) {
				sumY += assignLeafSlots(c, depth + 1, n.id);
			}
			const avgY = sumY / children.length;
			const fn: FlatNode = {
				data: n,
				x: 0,
				y: avgY,
				depth,
				parentId: parent,
			};
			flatNodes.push(fn);
			nodeMap.set(n.id, fn);
			return avgY;
		};

		assignLeafSlots(filteredTree, 0, null);

		const maxLeaves = Math.max(1, leafIndex);
		for (const fn of flatNodes) {
			if (projection === "vertical") {
				fn.x = (fn.y / maxLeaves) * (dimensions.width * 0.85) - dimensions.width * 0.38;
				fn.y = fn.depth * 95;
			} else if (projection === "radial") {
				const angle = (fn.y / maxLeaves) * Math.PI * 2;
				const radius = fn.depth * 110;
				fn.x = radius * Math.cos(angle - Math.PI / 2);
				fn.y = radius * Math.sin(angle - Math.PI / 2);
			} else {
				// Horizontal
				fn.x = fn.depth * 175;
				fn.y = (fn.y / maxLeaves) * (dimensions.height * 0.82);
			}
		}

		for (const fn of flatNodes) {
			if (fn.parentId) {
				const parent = nodeMap.get(fn.parentId);
				if (parent) {
					flatLinks.push({ source: parent, target: fn });
				}
			}
		}

		return { nodes: flatNodes, links: flatLinks };
	}, [filteredTree, projection, dimensions]);

	// Ancestor path highlighting
	const activePathIds = useMemo(() => {
		if (!hoveredNodeId) return null;
		const ids = new Set<string>();
		let curr = nodes.find((n) => n.data.id === hoveredNodeId);
		while (curr) {
			ids.add(curr.data.id);
			curr = curr.parentId ? nodes.find((n) => n.data.id === curr?.parentId) : undefined;
		}
		return ids;
	}, [hoveredNodeId, nodes]);

	// Node toggle click
	const toggleNode = (nodeData: TrieNode) => {
		const toggleInTree = (n: TrieNode): boolean => {
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
			for (const c of n.children ?? []) {
				if (toggleInTree(c)) return true;
			}
			for (const c of n._children ?? []) {
				if (toggleInTree(c)) return true;
			}
			return false;
		};

		const next = JSON.parse(JSON.stringify(tree));
		toggleInTree(next);
		setTree(next);
	};

	// Best Beam focus
	const focusBestBeam = () => {
		const next = JSON.parse(JSON.stringify(generateDefaultTrie()));
		const expandGreedy = (n: TrieNode) => {
			const all = [...(n.children || []), ...(n._children || [])];
			if (all.length === 0) return;

			const best = all.reduce((max, c) => (c.probability > max.probability ? c : max), all[0]);
			n.children = [best];
			n._children = all.filter((c) => c.id !== best.id);
			expandGreedy(best);
		};

		expandGreedy(next);
		setTree(next);
	};

	// Smooth mouse pan
	const handleMouseDown = (e: React.MouseEvent) => {
		isDragging.current = true;
		dragStart.current = { x: e.clientX - transform.x, y: e.clientY - transform.y };
	};

	const handleMouseMove = (e: React.MouseEvent) => {
		if (isDragging.current) {
			setTransform((prev) => ({
				...prev,
				x: e.clientX - dragStart.current.x,
				y: e.clientY - dragStart.current.y,
			}));
		}
	};

	const handleMouseUp = () => {
		isDragging.current = false;
	};

	// Smooth logarithmic mouse wheel zoom (not harsh)
	const handleWheel = (e: React.WheelEvent) => {
		e.preventDefault();
		const zoomFactor = Math.exp(-e.deltaY * 0.0015);
		const newK = Math.max(0.25, Math.min(4.0, transform.k * zoomFactor));

		const rect = wrapperRef.current?.getBoundingClientRect();
		if (!rect) return;
		const mouseX = e.clientX - rect.left;
		const mouseY = e.clientY - rect.top;
		const newX = mouseX - (mouseX - transform.x) * (newK / transform.k);
		const newY = mouseY - (mouseY - transform.y) * (newK / transform.k);

		setTransform({ x: newX, y: newY, k: newK });
	};

	// Precision port connections:
	// Horizontal: parent output port is at (source.x + 90, source.y), child input port is at (target.x - 10, target.y)
	// Vertical: parent output port is at (source.x + 40, source.y + 15), child input port is at (target.x + 40, target.y - 15)
	const getLinkPath = (s: FlatNode, t: FlatNode) => {
		if (projection === "vertical") {
			const startX = s.x + 40;
			const startY = s.y + 15;
			const endX = t.x + 40;
			const endY = t.y - 15;
			const midY = (startY + endY) / 2;
			return `M ${startX} ${startY} C ${startX} ${midY}, ${endX} ${midY}, ${endX} ${endY}`;
		}
		if (projection === "radial") {
			return `M ${s.x} ${s.y} L ${t.x} ${t.y}`;
		}
		const startX = s.x + 90;
		const startY = s.y;
		const endX = t.x - 10;
		const endY = t.y;
		const midX = (startX + endX) / 2;
		return `M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}`;
	};

	return (
		<div className="flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)">
			{/* Top Controls Toolbar - Flush with border-b */}
			<div className="flex h-10 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4">
				<div className="flex items-center gap-3">
					<Typography.Label size="s" tone="f4" weight="normal">
						PROJECTION
					</Typography.Label>
					<ToggleGroup
						name="projection"
						value={projection}
						onValueChange={(val) => setProjection(val as typeof projection)}
					>
						<ToggleGroupItem value="horizontal">Horizontal</ToggleGroupItem>
						<ToggleGroupItem value="vertical">Vertical</ToggleGroupItem>
						<ToggleGroupItem value="radial">Radial</ToggleGroupItem>
					</ToggleGroup>

					<div className="h-4 w-px bg-(--line)" />

					<Typography.Label size="s" tone="f4" weight="normal">
						COLOR
					</Typography.Label>
					<ToggleGroup
						name="color"
						value={colorMode}
						onValueChange={(val) => setColorMode(val as typeof colorMode)}
					>
						<ToggleGroupItem value="threshold">Threshold</ToggleGroupItem>
						<ToggleGroupItem value="gradient">Gradient</ToggleGroupItem>
					</ToggleGroup>
				</div>

				<div className="flex items-center gap-3">
					<Slider.Field
						leading={
							<Typography.Label size="s" tone="f4" weight="normal">
								PRUNE
							</Typography.Label>
						}
						trailing={
							<span className="w-10 text-right text-(--acc) font-bold">
								{(minProb * 100).toFixed(0)}%
							</span>
						}
					>
						<Slider
							min={0}
							max={0.4}
							step={0.01}
							value={minProb}
							onChange={(e) => setMinProb(parseFloat(e.target.value))}
							className="w-24"
						/>
					</Slider.Field>

					<Button
						variant="outline"
						size="s"
						onClick={focusBestBeam}
						className="text-(--acc)"
					>
						BEST BEAM
					</Button>
				</div>
			</div>

			{/* Center SVG Tree Visualizer Canvas - Rendered on black screen surface bg-(--sunken) */}
			<div
				ref={wrapperRef}
				onMouseDown={handleMouseDown}
				onMouseMove={handleMouseMove}
				onMouseUp={handleMouseUp}
				onMouseLeave={handleMouseUp}
				onWheel={handleWheel}
				className="relative flex-1 min-h-0 cursor-grab overflow-hidden bg-(--sunken) active:cursor-grabbing"
			>
				{/* Canvas Grid Background */}
				<div
					className="absolute inset-0 pointer-events-none opacity-20"
					style={{
						backgroundImage:
							"linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px)",
						backgroundSize: "24px 24px",
						transform: `translate(${transform.x % 24}px, ${transform.y % 24}px)`,
					}}
				/>

				<svg ref={svgRef} className="absolute inset-0 block h-full w-full">
					<g transform={`translate(${transform.x}, ${transform.y}) scale(${transform.k})`}>
						{/* Edges / Links with motion animation */}
						<g className="links">
							<AnimatePresence>
								{links.map((link) => {
									const path = getLinkPath(link.source, link.target);
									const isActive = activePathIds
										? activePathIds.has(link.target.data.id)
										: true;
									const prob = link.target.data.probability;
									const strokeColor =
										colorMode === "gradient"
											? prob > 0.15
												? "var(--acc)"
												: "var(--line2)"
											: prob > 0.2
												? "var(--acc)"
												: "var(--line2)";

									const textX = (link.source.x + link.target.x) / 2 + (projection === "vertical" ? 40 : 40);
									const textY = (link.source.y + link.target.y) / 2 - 4;

									return (
										<motion.g
											key={`link-${link.source.data.id}-${link.target.data.id}`}
											initial={{ opacity: 0 }}
											animate={{ opacity: isActive ? 1 : 0.15 }}
											exit={{ opacity: 0 }}
											transition={{ duration: 0.35 }}
										>
											<motion.path
												d={path}
												fill="none"
												stroke={strokeColor}
												strokeWidth={Math.max(1.5, prob * 6)}
												strokeOpacity={isActive ? 0.85 : 0.2}
												transition={{ duration: 0.35 }}
											/>
											{link.target.data.tokens && (
												<text
													x={textX}
													y={textY}
													fill="var(--f4)"
													fontSize="9px"
													textAnchor="middle"
													className="pointer-events-none select-none font-mono"
												>
													[{link.target.data.tokens.join(", ")}]
												</text>
											)}
										</motion.g>
									);
								})}
							</AnimatePresence>
						</g>

						{/* Tree Nodes with motion animation */}
						<g className="nodes">
							<AnimatePresence>
								{nodes.map((node) => {
									const data = node.data;
									const hasChildren = Boolean(data.children || data._children);
									const isCollapsed = Boolean(data._children);
									const isActive = activePathIds ? activePathIds.has(data.id) : true;
									const isHighProb = data.probability > 0.2;

									const isVertical = projection === "vertical";
									const parentPort = isVertical ? { cx: 40, cy: -15 } : { cx: -10, cy: 0 };
									const childPort = isVertical ? { cx: 40, cy: 15 } : { cx: 90, cy: 0 };

									return (
										<motion.g
											key={`node-${data.id}`}
											initial={{ opacity: 0, x: node.x, y: node.y }}
											animate={{ opacity: isActive ? 1 : 0.2, x: node.x, y: node.y }}
											exit={{ opacity: 0 }}
											transition={{ duration: 0.35 }}
											onClick={(e) => {
												e.stopPropagation();
												if (hasChildren) toggleNode(data);
											}}
											onMouseEnter={(e) => {
												setHoveredNodeId(data.id);
												setTooltip({ data, x: e.clientX, y: e.clientY });
											}}
											onMouseMove={(e) => {
												setTooltip({ data, x: e.clientX, y: e.clientY });
											}}
											onMouseLeave={() => {
												setHoveredNodeId(null);
												setTooltip(null);
											}}
											className={hasChildren ? "cursor-pointer" : "cursor-default"}
										>
											{/* Node Card Box */}
											<rect
												x={-10}
												y={-15}
												width={100}
												height={30}
												rx={3}
												fill="var(--surface)"
												stroke={
													data.state === "POLICY CHOICE"
														? "var(--up)"
														: isHighProb
															? "var(--acc)"
															: "var(--line2)"
												}
												strokeWidth={isHighProb ? 1.5 : 1}
												className="transition-colors hover:stroke-(--acc)"
											/>

											{/* Left Input Port */}
											{node.parentId && (
												<circle
													cx={parentPort.cx}
													cy={parentPort.cy}
													r={3}
													fill="var(--surface)"
													stroke="var(--f4)"
													strokeWidth={1.5}
												/>
											)}

											{/* Right Output Port */}
											{(hasChildren || isCollapsed) && (
												<circle
													cx={childPort.cx}
													cy={childPort.cy}
													r={isCollapsed ? 4 : 3}
													fill={isCollapsed ? "var(--acc)" : "var(--surface)"}
													stroke={isCollapsed ? "var(--acc)" : "var(--f4)"}
													strokeWidth={1.5}
												/>
											)}

											{/* Node Prefix Text */}
											<text
												x={0}
												y={0}
												dy="0.32em"
												fill={
													data.state === "POLICY CHOICE"
														? "var(--up)"
														: isHighProb
															? "var(--acc)"
															: "var(--f1)"
												}
												fontSize="11.5px"
												fontWeight="bold"
												className="pointer-events-none select-none"
											>
												{data.prefix}
											</text>

											{/* Probability Tag */}
											<text
												x={80}
												y={-20}
												fill="var(--f4)"
												fontSize="9px"
												textAnchor="end"
												className="pointer-events-none select-none"
											>
												{(data.probability * 100).toFixed(0)}%
											</text>

											{/* State Badge */}
											{data.state && (
												<text
													x={0}
													y={-20}
													fill={
														data.state === "POLICY CHOICE"
															? "var(--up)"
															: data.state === "EVALUATED"
																? "var(--acc)"
																: "var(--info)"
													}
													fontSize="8px"
													fontWeight="bold"
													className="pointer-events-none select-none tracking-wider"
												>
													{data.state}
												</text>
											)}
										</motion.g>
									);
								})}
							</AnimatePresence>
						</g>
					</g>
				</svg>

				{/* Floating Tooltip */}
				{tooltip && (
					<div
						className="fixed z-50 rounded border border-(--line) bg-(--raised) p-2.5 font-mono text-[10px] text-(--f2) shadow-xl pointer-events-none"
						style={{ left: tooltip.x + 14, top: tooltip.y + 14, minWidth: 200 }}
					>
						<div className="flex items-center justify-between border-b border-(--line) pb-1.5">
							<span className="font-bold text-(--acc)">{tooltip.data.prefix}</span>
							{tooltip.data.state && (
								<Badge
									size="s"
									variant={
										tooltip.data.state === "POLICY CHOICE"
											? "success"
											: tooltip.data.state === "EVALUATED"
												? "warning"
												: "info"
									}
									label={tooltip.data.state}
								/>
							)}
						</div>
						<div className="mt-1.5 flex justify-between">
							<span className="text-(--f4)">Sequence Prob:</span>
							<span className="font-bold text-(--f1)">
								{(tooltip.data.probability * 100).toFixed(1)}%
							</span>
						</div>
						{tooltip.data.stepProbability && (
							<div className="flex justify-between">
								<span className="text-(--f4)">Step Prob:</span>
								<span>{(tooltip.data.stepProbability * 100).toFixed(1)}%</span>
							</div>
						)}
						{tooltip.data.tokens && (
							<div className="mt-1 border-t border-(--line)/40 pt-1 text-(--f3)">
								Tokens: [{tooltip.data.tokens.join(", ")}]
							</div>
						)}
					</div>
				)}

				{/* Bottom Right Zoom Overlay Controls */}
				<div className="absolute bottom-3 right-3 flex items-center gap-1.5">
					<Button
						variant="outline"
						size="s"
						onClick={() => setTransform((prev) => ({ ...prev, k: prev.k * 1.2 }))}
					>
						+
					</Button>
					<Button
						variant="outline"
						size="s"
						onClick={() => setTransform((prev) => ({ ...prev, k: prev.k * 0.8 }))}
					>
						-
					</Button>
					<Button
						variant="outline"
						size="s"
						onClick={() => setTransform({ x: 60, y: dimensions.height / 2, k: 1 })}
					>
						RESET
					</Button>
				</div>
			</div>

			{/* Bottom Tray: Feasible Actions Table - Flush with border-t */}
			<div className="flex h-36 shrink-0 flex-col border-t border-(--line) bg-(--surface) overflow-hidden">
				<div className="flex h-7 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[10px] text-(--f4)">
					<span className="font-bold text-(--f2)">FEASIBLE ACTIONS AT THIS PRECURSOR IMPULSE</span>
					<span>Beam search ranked outcomes</span>
				</div>

				<div className="flex-1 overflow-hidden p-2">
					<table className="w-full text-left text-[11px]">
						<thead>
							<tr className="border-b border-(--line) text-(--f4)">
								<th className="w-12 pb-1 px-3 font-normal">Rank</th>
								<th className="w-32 pb-1 px-3 font-normal">Action</th>
								<th className="pb-1 px-3 font-normal">Prefix Sequence</th>
								<th className="pb-1 px-3 text-right font-normal">Probability</th>
								<th className="pb-1 px-3 text-right font-normal">State</th>
							</tr>
						</thead>
						<tbody>
							<tr className="border-b border-(--line)/40 hover:bg-(--raised)">
								<td className="py-1 px-3 text-(--acc) font-bold">1</td>
								<td className="py-1 px-3 text-(--acc) font-bold">wait</td>
								<td className="py-1 px-3 text-(--f3)">ROOT / wait / 100ms</td>
								<td className="py-1 px-3 text-right text-(--f1) font-bold">30.0%</td>
								<td className="py-1 px-3 text-right">
									<Badge size="s" variant="warning" label="EVALUATED" />
								</td>
							</tr>
							<tr className="border-b border-(--line)/40 hover:bg-(--raised)">
								<td className="py-1 px-3 text-(--up) font-bold">2</td>
								<td className="py-1 px-3 text-(--up) font-bold">enter · 1/8</td>
								<td className="py-1 px-3 text-(--f3)">ROOT / enter / 1/8 / @ask</td>
								<td className="py-1 px-3 text-right text-(--f1) font-bold">12.0%</td>
								<td className="py-1 px-3 text-right">
									<Badge size="s" variant="success" label="POLICY CHOICE" />
								</td>
							</tr>
							<tr className="hover:bg-(--raised)">
								<td className="py-1 px-3 text-(--f3)">3</td>
								<td className="py-1 px-3 text-(--f2)">enter · 1/8</td>
								<td className="py-1 px-3 text-(--f3)">ROOT / enter / 1/8 / @bid</td>
								<td className="py-1 px-3 text-right text-(--f1) font-bold">8.0%</td>
								<td className="py-1 px-3 text-right">
									<Badge size="s" variant="info" label="ESTIMATED" />
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
};
