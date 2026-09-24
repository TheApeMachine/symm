import { AnimatePresence, motion } from "motion/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "#/components/ui/button";
import { Slider } from "#/components/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "#/components/ui/toggle-group";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Badge } from "./badge";

export interface TrieNode {
	id: string;
	prefix: string;
	probability: number;
	stepProbability?: number;
	tokens?: string[];
	// label is what was recorded at this prefix most often (an action), share
	// its part of the endings there, visits the paths through the node.
	label?: string;
	share?: number;
	visits?: number;
	state?: "EVALUATED" | "POLICY CHOICE" | "ESTIMATED";
	isEnd?: boolean;
	children?: TrieNode[];
	_children?: TrieNode[];
}

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

/* Layout spacing, in SVG units before zoom: a node box is 100 by 30. */
const SIBLING_GAP = 56;
const GENERATION_GAP = 200;

/* actionTone colours a recorded action the way the rest of the surface does. */
const actionTone = (label?: string) => {
	if (label === "ENTER") return "var(--up)";
	if (label === "EXIT") return "var(--down)";
	return "var(--f1)";
};

export interface TrieCandidate {
	id: string;
	rank: number;
	action: string;
	prefix: string;
	probability: number;
	state: string;
}
export interface TrieViewProps {
	root?: TrieNode | null;
	candidates?: TrieCandidate[] | null;
	className?: string;
}

/* Chooses the path to the highest supplied terminal probability, for display only. */
export const selectTriePath = (root: TrieNode): Set<string> => {
	const visit = (
		node: TrieNode,
		ancestors: string[],
	): { probability: number; path: string[] } => {
		const path = [...ancestors, node.id];
		const children = node.children ?? [];
		if (!children.length) return { probability: node.probability, path };
		let selected = node.isEnd
			? { probability: node.probability, path }
			: visit(children[0], path);
		for (const child of children.slice(node.isEnd ? 0 : 1)) {
			const candidate = visit(child, path);
			if (candidate.probability > selected.probability) selected = candidate;
		}
		return selected;
	};
	return new Set(visit(root, []).path);
};

export const TrieView = ({
	root,
	candidates: suppliedCandidates,
	className,
}: TrieViewProps) => {
	const candidates = suppliedCandidates ?? [];
	const svgRef = useRef<SVGSVGElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const [dimensions, setDimensions] = useState({ width: 800, height: 500 });

	const [projection, setProjection] = useState<
		"horizontal" | "vertical" | "radial"
	>("horizontal");
	const [minProb, setMinProb] = useState(0.0);

	const [presentation, setPresentation] = useState<{
		root: TrieNode | null | undefined;
		collapsed: Set<string>;
		best: boolean;
	}>({ root, collapsed: new Set(), best: false });
	const collapsed =
		presentation.root === root ? presentation.collapsed : new Set<string>();
	const bestOnly = presentation.root === root && presentation.best;
	const preferredIds = bestOnly && root ? selectTriePath(root) : null;
	const [transform, setTransform] = useState({ x: 60, y: 250, k: 1 });
	const isDragging = useRef(false);
	const dragStart = useRef({ x: 0, y: 0 });

	const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
	const [selection, setTooltip] = useState<{
		root: TrieNode | null | undefined;
		data: TrieNode;
		x: number;
		y: number;
	} | null>(null);

	const tooltip = selection?.root === root ? selection : null;

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

	const filterNode = (node: TrieNode): TrieNode | null => {
		if (node.probability < minProb) return null;
		const children = (node.children ?? [])
			.map(filterNode)
			.filter((child): child is TrieNode => child !== null);
		if (collapsed.has(node.id))
			return { ...node, children: undefined, _children: children };
		if (preferredIds && children.length) {
			return {
				...node,
				children: children.filter((child) => preferredIds.has(child.id)),
				_children: children.filter((child) => !preferredIds.has(child.id)),
			};
		}
		return { ...node, children, _children: undefined };
	};
	const filteredTree = root ? filterNode(root) : null;

	// Compute tree layout
	const { nodes, links } = useMemo(() => {
		const flatNodes: FlatNode[] = [];
		const flatLinks: FlatLink[] = [];
		const nodeMap = new Map<string, FlatNode>();

		let leafIndex = 0;
		const assignLeafSlots = (
			n: TrieNode,
			depth: number,
			parent: string | null,
		) => {
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

		if (filteredTree) assignLeafSlots(filteredTree, 0, null);

		// Siblings and generations keep a fixed spacing so labels never
		// collide; a large trie is panned and zoomed, not squeezed.
		const maxLeaves = Math.max(1, leafIndex);
		const rootSlot = filteredTree ? (nodeMap.get(filteredTree.id)?.y ?? 0) : 0;
		for (const fn of flatNodes) {
			const slot = fn.y - rootSlot;
			if (projection === "vertical") {
				fn.x = slot * SIBLING_GAP * 2.2;
				fn.y = fn.depth * GENERATION_GAP * 0.55;
			} else if (projection === "radial") {
				const angle = (fn.y / maxLeaves) * Math.PI * 2;
				const radius = fn.depth * GENERATION_GAP * 0.7;
				fn.x = radius * Math.cos(angle - Math.PI / 2);
				fn.y = radius * Math.sin(angle - Math.PI / 2);
			} else {
				fn.x = fn.depth * GENERATION_GAP;
				fn.y = slot * SIBLING_GAP;
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
	}, [filteredTree, projection]);

	// Ancestor path highlighting
	const activePathIds = useMemo(() => {
		if (!hoveredNodeId) return null;
		const ids = new Set<string>();
		let curr = nodes.find((n) => n.data.id === hoveredNodeId);
		while (curr) {
			ids.add(curr.data.id);
			curr = curr.parentId
				? nodes.find((n) => n.data.id === curr?.parentId)
				: undefined;
		}
		return ids;
	}, [hoveredNodeId, nodes]);

	const toggleNode = (node: TrieNode) => {
		const next = new Set(collapsed);
		if (next.has(node.id)) next.delete(node.id);
		else next.add(node.id);
		setPresentation({ root, collapsed: next, best: false });
	};
	const focusBestBeam = () =>
		setPresentation({ root, collapsed: new Set(), best: !bestOnly });

	// Smooth mouse pan
	const handleMouseDown = (e: React.MouseEvent) => {
		isDragging.current = true;
		dragStart.current = {
			x: e.clientX - transform.x,
			y: e.clientY - transform.y,
		};
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
		<div
			className={cn(
				"flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)",
				className,
			)}
		>
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
							max={1}
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
						{bestOnly ? "ALL PATHS" : "HIGHEST PROBABILITY PATH"}
					</Button>
				</div>
			</div>

			{/* Pan with the pointer or the keyboard-accessible controls below. */}
			<div
				ref={wrapperRef}
				role="application"
				aria-label="Trie pan and zoom"
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

				{!root && (
					<Typography.Mono className="relative p-4">
						No recorded trie
					</Typography.Mono>
				)}
				{root && nodes.length === 0 && (
					<Typography.Mono className="relative p-4">
						No nodes match the display filter
					</Typography.Mono>
				)}
				<svg
					ref={svgRef}
					role="img"
					aria-label="Recorded trie"
					className="absolute inset-0 block h-full w-full"
				>
					<title>Recorded trie</title>
					<g
						transform={`translate(${transform.x}, ${transform.y}) scale(${transform.k})`}
					>
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
										link.target.data.state === "POLICY CHOICE"
											? "var(--up)"
											: "var(--line2)";

									// The step's tokens sit just before the node they lead to,
									// so siblings fanning out of one parent never share a spot.
									const textX =
										projection === "horizontal"
											? link.target.x - 16
											: link.target.x + 40;
									const textY =
										projection === "horizontal"
											? link.target.y - 6
											: link.target.y - 22;

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
													textAnchor={
														projection === "horizontal" ? "end" : "middle"
													}
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
									const hasChildren = Boolean(
										data.children?.length || data._children?.length,
									);
									const isCollapsed = Boolean(data._children?.length);
									const isActive = activePathIds
										? activePathIds.has(data.id)
										: true;
									const isPolicyChoice = data.state === "POLICY CHOICE";

									const isVertical = projection === "vertical";
									const parentPort = isVertical
										? { cx: 40, cy: -15 }
										: { cx: -10, cy: 0 };
									const childPort = isVertical
										? { cx: 40, cy: 15 }
										: { cx: 90, cy: 0 };

									return (
										<motion.g
											key={`node-${data.id}`}
											role="button"
											tabIndex={0}
											aria-label={
												node.parentId
													? `Toggle ${data.label ?? "·"} at ${data.prefix}`
													: "Toggle root"
											}
											onKeyDown={(event) => {
												if (
													(event.key === "Enter" || event.key === " ") &&
													hasChildren
												) {
													event.preventDefault();
													toggleNode(data);
												}
											}}
											initial={{ opacity: 0, x: node.x, y: node.y }}
											animate={{
												opacity: isActive ? 1 : 0.2,
												x: node.x,
												y: node.y,
											}}
											exit={{ opacity: 0 }}
											transition={{ duration: 0.35 }}
											onClick={(e) => {
												e.stopPropagation();
												if (hasChildren) toggleNode(data);
											}}
											onMouseEnter={(e) => {
												setHoveredNodeId(data.id);
												setTooltip({ root, data, x: e.clientX, y: e.clientY });
											}}
											onMouseMove={(e) => {
												setTooltip({ root, data, x: e.clientX, y: e.clientY });
											}}
											onMouseLeave={() => {
												setHoveredNodeId(null);
												setTooltip(null);
											}}
											className={
												hasChildren ? "cursor-pointer" : "cursor-default"
											}
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
														: isPolicyChoice
															? "var(--acc)"
															: "var(--line2)"
												}
												strokeWidth={isPolicyChoice ? 1.5 : 1}
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

											{/* The action recorded at this prefix */}
											<text
												x={0}
												y={0}
												dy="0.32em"
												fill={
													node.parentId ? actionTone(data.label) : "var(--f3)"
												}
												fontSize="11.5px"
												fontWeight="bold"
												className="pointer-events-none select-none"
											>
												{node.parentId ? (data.label ?? "·") : "ROOT"}
											</text>
											{data.share !== undefined && (
												<text
													x={84}
													y={0}
													dy="0.32em"
													fill="var(--f3)"
													fontSize="9px"
													textAnchor="end"
													className="pointer-events-none select-none"
												>
													{(data.share * 100).toFixed(0)}%
												</text>
											)}

											{/* Share of all paths that pass through */}
											<text
												x={80}
												y={-20}
												fill="var(--f4)"
												fontSize="9px"
												textAnchor="end"
												className="pointer-events-none select-none"
											>
												{(data.probability * 100).toFixed(1)}%
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
							<span className="font-bold text-(--acc)">
								{tooltip.data.label ?? "ROOT"}
							</span>
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
						<div className="mt-1.5 break-all text-(--f3)">
							{tooltip.data.prefix}
						</div>
						{tooltip.data.share !== undefined && (
							<div className="mt-1.5 flex justify-between">
								<span className="text-(--f4)">Recorded here:</span>
								<span
									className="font-bold"
									style={{ color: actionTone(tooltip.data.label) }}
								>
									{tooltip.data.label} · {(tooltip.data.share * 100).toFixed(1)}
									%
								</span>
							</div>
						)}
						{tooltip.data.visits !== undefined && (
							<div className="flex justify-between">
								<span className="text-(--f4)">Visits:</span>
								<span>{tooltip.data.visits.toLocaleString()}</span>
							</div>
						)}
						<div className="flex justify-between">
							<span className="text-(--f4)">Sequence Prob:</span>
							<span className="font-bold text-(--f1)">
								{(tooltip.data.probability * 100).toFixed(1)}%
							</span>
						</div>
						{tooltip.data.stepProbability !== undefined && (
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
					{[
						{ label: "Pan left", glyph: "←", x: -20, y: 0 },
						{ label: "Pan right", glyph: "→", x: 20, y: 0 },
						{ label: "Pan up", glyph: "↑", x: 0, y: -20 },
						{ label: "Pan down", glyph: "↓", x: 0, y: 20 },
					].map((direction) => (
						<Button
							key={direction.label}
							aria-label={direction.label}
							variant="outline"
							size="s"
							onClick={() =>
								setTransform((previous) => ({
									...previous,
									x: previous.x + direction.x,
									y: previous.y + direction.y,
								}))
							}
						>
							{direction.glyph}
						</Button>
					))}
					<Button
						variant="outline"
						size="s"
						onClick={() =>
							setTransform((prev) => ({ ...prev, k: prev.k * 1.2 }))
						}
					>
						+
					</Button>
					<Button
						variant="outline"
						size="s"
						onClick={() =>
							setTransform((prev) => ({ ...prev, k: prev.k * 0.8 }))
						}
					>
						-
					</Button>
					<Button
						variant="outline"
						size="s"
						onClick={() =>
							setTransform({ x: 60, y: dimensions.height / 2, k: 1 })
						}
					>
						RESET
					</Button>
				</div>
			</div>

			{/* Bottom Tray: Feasible Actions Table - Flush with border-t */}
			<div className="flex h-36 shrink-0 flex-col border-t border-(--line) bg-(--surface) overflow-hidden">
				<div className="flex h-7 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[10px] text-(--f4)">
					<span className="font-bold text-(--f2)">
						FEASIBLE ACTIONS AT THIS PRECURSOR IMPULSE
					</span>
					<span>Producer-ranked outcomes</span>
				</div>

				<div className="flex-1 overflow-hidden p-2">
					<table className="w-full text-left text-[11px]">
						<thead>
							<tr className="border-b border-(--line) text-(--f4)">
								<th className="w-12 pb-1 px-3 font-normal">Rank</th>
								<th className="w-32 pb-1 px-3 font-normal">Action</th>
								<th className="pb-1 px-3 font-normal">Prefix Sequence</th>
								<th className="pb-1 px-3 text-right font-normal">
									Probability
								</th>
								<th className="pb-1 px-3 text-right font-normal">State</th>
							</tr>
						</thead>
						<tbody>
							{candidates.length === 0 && (
								<tr>
									<td colSpan={5}>No supplied action rankings</td>
								</tr>
							)}
							{candidates.map((candidate) => (
								<tr key={candidate.id}>
									<td className="p-2">{candidate.rank}</td>
									<td>{candidate.action}</td>
									<td>{candidate.prefix}</td>
									<td>{(candidate.probability * 100).toFixed(1)}%</td>
									<td>{candidate.state}</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
};
