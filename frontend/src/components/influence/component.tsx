import { useEffect, useMemo, useRef, useState } from "react";
import {
	ensureLineageLoaded,
	type LineageStatus,
	lineageStatusOf,
	type ProducerRow,
	readLineage,
	subscribeLineage,
} from "#/components/terminal/lineage-report";
import { Chip } from "#/components/ui/chip";
import { Flex } from "#/components/ui/flex";
import { Icon } from "#/components/ui/icon";
import { Toolbar } from "#/components/ui/toolbar";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
InfluenceField is the original force-directed field visualization, unchanged,
now sourced from the same static tools/metriclineage report Metric lineage
reads (frontend/src/components/terminal/lineage-report.ts) instead of the
live websocket regression graph (graphStore/graph.Solver) it used to run on.
There is no websocket subscription and no per-tick recomputation of anything
here — the animation loop below only replays the field's existing idle-noise
motion against a snapshot that's fetched once and doesn't change until the
underlying JSON is regenerated (`go run ./tools/metriclineage . frontend/public/metric-lineage.json`,
wired into `make run`).

Node = one metric (a ProducerRow). Edge = one metric's "fine" (named)
consumer link to another metric that shares that same consumer — e.g. two
metrics both bound by the same advisor are drawn connected, mirroring how the
original regression graph connected metrics that predicted one another.
Weight (the field's "influence," fed into every ring/arrow/hub radius below)
is no longer a regression coefficient rank; it's a signed rank of maturity
for used metrics and a fixed negative rank for dead ones — used pulls the
field toward --up, dead pulls it toward --down, exactly like a positive vs.
negative coefficient used to.
*/

type InfluencedNode = {
	id: string;
	symbol?: string;
	source?: string;
	status: LineageStatus;
	value?: number;
	confidence?: number;
	weight: number;
	edgeCount: number;
	x: number;
	y: number;
	producer: ProducerRow;
};

type RankedEdge = {
	from: string;
	to: string;
	relation?: string;
	weight: number;
	confidence: number;
	reason?: string;
	rank: number;
};

/*
rankWeight converts a list of signed raw values into signed [-1,1] rank
scores: each value's fractional position among the sorted |value|s of the
whole set, keeping its original sign. Ties share the same rank.
*/
const rankWeight = (values: number[]): number[] => {
	if (values.length === 0) return [];
	if (values.length === 1) return [Math.sign(values[0])];

	const order = values
		.map((value, index) => ({ index, abs: Math.abs(value) }))
		.sort((a, b) => a.abs - b.abs);

	const percentile = new Array<number>(values.length);
	let i = 0;
	while (i < order.length) {
		let j = i;
		while (j + 1 < order.length && order[j + 1].abs === order[i].abs) j++;
		const rank = (i + j) / 2 / (order.length - 1);
		for (let k = i; k <= j; k++) percentile[order[k].index] = rank;
		i = j + 1;
	}

	return values.map((value, index) => percentile[index] * Math.sign(value));
};

/*
hashUnit turns a node id into a stable [0,1) value. Positions must be
deterministic across re-renders (no Math.random()), and the layout below only
needs a stable starting angle per id, not real randomness.
*/
const hashUnit = (id: string): number => {
	let hash = 2166136261;
	for (let i = 0; i < id.length; i++) {
		hash ^= id.charCodeAt(i);
		hash = Math.imul(hash, 16777619);
	}
	return ((hash >>> 0) % 10000) / 10000;
};

/*
layoutNodes runs a short, deterministic force simulation: every node repels
every other node, and every edge pulls its two endpoints together in
proportion to its own rank — a stronger link draws its pair visibly closer.
*/
const layoutNodes = (
	nodeIds: string[],
	edges: RankedEdge[],
	width: number,
	height: number,
): Map<string, { x: number; y: number }> => {
	const cx = width / 2;
	const cy = height / 2;
	const radius = Math.min(width, height) * 0.36;

	const nodePositions = nodeIds.map((id, index) => {
		const angle = hashUnit(id) * Math.PI * 2 + index * 0.0001;
		return {
			id,
			position: {
				x: cx + Math.cos(angle) * radius,
				y: cy + Math.sin(angle) * radius,
			},
		};
	});

	if (nodeIds.length < 2) {
		return new Map(nodePositions.map(({ id, position }) => [id, position]));
	}

	const indexOf = new Map(nodeIds.map((id, index) => [id, index]));
	const edgePairs = edges
		.map((edge) => ({
			i: indexOf.get(edge.from),
			j: indexOf.get(edge.to),
			pull: Math.abs(edge.rank) * 0.02 + 0.002,
		}))
		.filter(
			(edge): edge is { i: number; j: number; pull: number } =>
				edge.i !== undefined && edge.j !== undefined,
		);

	const iterations = 120;
	const minSeparation = Math.max(60, Math.min(width, height) * 0.09);

	for (let step = 0; step < iterations; step++) {
		const forces = nodePositions.map(() => ({ fx: 0, fy: 0 }));

		for (let i = 0; i < nodePositions.length; i++) {
			for (let j = i + 1; j < nodePositions.length; j++) {
				const a = nodePositions[i].position;
				const b = nodePositions[j].position;
				const dx = a.x - b.x;
				const dy = a.y - b.y;
				const distSq = dx * dx + dy * dy + 0.01;
				const dist = Math.sqrt(distSq);
				if (dist >= minSeparation * 2.4) continue;

				const push = (minSeparation * minSeparation) / distSq;
				forces[i].fx += (dx / dist) * push;
				forces[i].fy += (dy / dist) * push;
				forces[j].fx -= (dx / dist) * push;
				forces[j].fy -= (dy / dist) * push;
			}
		}

		for (const edge of edgePairs) {
			const { i, j } = edge;
			const a = nodePositions[i].position;
			const b = nodePositions[j].position;
			const dx = b.x - a.x;
			const dy = b.y - a.y;
			forces[i].fx += dx * edge.pull;
			forces[i].fy += dy * edge.pull;
			forces[j].fx -= dx * edge.pull;
			forces[j].fy -= dy * edge.pull;
		}

		for (let i = 0; i < nodePositions.length; i++) {
			const position = nodePositions[i].position;
			const force = forces[i];
			const centerPull = 0.01;
			position.x += force.fx - (position.x - cx) * centerPull;
			position.y += force.fy - (position.y - cy) * centerPull;
			position.x = Math.min(width - 24, Math.max(24, position.x));
			position.y = Math.min(height - 24, Math.max(24, position.y));
		}
	}

	return new Map(nodePositions.map(({ id, position }) => [id, position]));
};

/*
buildInfluencedNodes turns the static lineage report's producers into the
same {nodes, edges} shape the field's rendering code has always consumed.
Two metrics get an edge when they share at least one "fine" (named) consumer
— the closest honest analogue to the original regression graph's "these two
predict one another" edge, since the lineage data only records
metric-to-consumer links, not metric-to-metric ones.
*/
const buildInfluencedNodes = (
	producers: ProducerRow[],
	width: number,
	height: number,
): { nodes: InfluencedNode[]; edges: RankedEdge[] } => {
	if (producers.length === 0) {
		return { nodes: [], edges: [] };
	}

	const byConsumer = new Map<string, ProducerRow[]>();
	for (const producer of producers) {
		for (const consumer of producer.consumers) {
			if (consumer.kind !== "fine") continue;
			const list = byConsumer.get(consumer.consumer) ?? [];
			list.push(producer);
			byConsumer.set(consumer.consumer, list);
		}
	}

	const rawEdges: Array<{ from: string; to: string; reason: string; weight: number }> = [];
	const seenPairs = new Set<string>();

	for (const [consumer, rows] of byConsumer) {
		for (let i = 0; i < rows.length; i++) {
			for (let j = i + 1; j < rows.length; j++) {
				const pairKey = [rows[i].id, rows[j].id].sort().join("|");
				if (seenPairs.has(pairKey)) continue;
				seenPairs.add(pairKey);
				rawEdges.push({
					from: rows[i].id,
					to: rows[j].id,
					reason: `shared consumer: ${consumer}`,
					weight: 1,
				});
			}
		}
	}

	const ranks = rankWeight(rawEdges.map((edge) => edge.weight));
	const edges: RankedEdge[] = rawEdges.map((edge, index) => ({
		...edge,
		confidence: 0.6,
		rank: ranks[index],
	}));

	const connected = new Set<string>();
	const edgeCount = new Map<string, number>();
	for (const edge of edges) {
		connected.add(edge.from);
		connected.add(edge.to);
		edgeCount.set(edge.from, (edgeCount.get(edge.from) ?? 0) + 1);
		edgeCount.set(edge.to, (edgeCount.get(edge.to) ?? 0) + 1);
	}

	// Isolated metrics (no shared consumer with anything else, including
	// every dead metric by definition) still render — the field is meant to
	// show them, not hide them for lack of an edge.
	for (const producer of producers) {
		connected.add(producer.id);
	}

	const nodeIds = [...connected].sort();
	const positions = layoutNodes(nodeIds, edges, width, height);
	const byId = new Map(producers.map((p) => [p.id, p]));

	const nodes: InfluencedNode[] = nodeIds.map((id) => {
		const producer = byId.get(id);
		const position = positions.get(id) ?? { x: width / 2, y: height / 2 };
		const status: LineageStatus = producer ? (lineageStatusOf(producer) ?? "used") : "dead";
		// used -> positive weight (pulls the field toward --up), dead ->
		// negative (toward --down), kernelOnly sits near zero (weak signal:
		// something reads the whole kernel, but nothing names this metric).
		const weight = status === "used" ? 1 : status === "kernelOnly" ? 0.15 : -1;

		return {
			id,
			symbol: undefined,
			source: producer?.source,
			status,
			value: undefined,
			confidence: producer ? (producer.dead ? 0 : 1) : 0,
			weight,
			edgeCount: edgeCount.get(id) ?? 0,
			x: position.x,
			y: position.y,
			producer: producer as ProducerRow,
		};
	});

	return { nodes, edges };
};

const shortLabel = (node: InfluencedNode): string => `${node.source ?? "?"} · ${node.producer?.metric ?? node.id}`;

export const InfluenceField = () => {
	const [dimensions, setDimensions] = useState({ width: 800, height: 500 });
	const containerRef = useRef<HTMLDivElement>(null);

	const [activeNodeId, setActiveNodeId] = useState<string | null>(null);
	const [phase, setPhase] = useState(0);
	const [isPlaying, setIsPlaying] = useState(true);

	const lastTimeRef = useRef(Date.now());
	const animationFrameRef = useRef<number | null>(null);
	const [, setVersion] = useState(0);

	useEffect(() => {
		if (!containerRef.current) return;
		const observer = new ResizeObserver((entries) => {
			if (!entries || entries.length === 0) return;
			const { width, height } = entries[0].contentRect;
			setDimensions({ width, height });
		});
		observer.observe(containerRef.current);
		return () => observer.disconnect();
	}, []);

	useEffect(() => {
		ensureLineageLoaded();
		const notify = () => setVersion((value) => value + 1);
		return subscribeLineage(notify);
	}, []);

	useEffect(() => {
		const tick = () => {
			if (!isPlaying) return;
			const now = Date.now();
			const delta = (now - lastTimeRef.current) * 0.002;
			lastTimeRef.current = now;
			setPhase((prev) => (prev + delta) % (Math.PI * 2));
			animationFrameRef.current = requestAnimationFrame(tick);
		};

		if (isPlaying) {
			lastTimeRef.current = Date.now();
			animationFrameRef.current = requestAnimationFrame(tick);
		}

		return () => {
			if (animationFrameRef.current) {
				cancelAnimationFrame(animationFrameRef.current);
			}
		};
	}, [isPlaying]);

	const { report } = readLineage();
	const producers = report?.producers ?? [];

	const { nodes, edges } = useMemo(
		() => buildInfluencedNodes(producers, dimensions.width, dimensions.height),
		[producers, dimensions.width, dimensions.height],
	);

	useEffect(() => {
		if (activeNodeId !== null && !nodes.some((node) => node.id === activeNodeId)) {
			setActiveNodeId(nodes[0]?.id ?? null);
		} else if (activeNodeId === null && nodes.length > 0) {
			setActiveNodeId(nodes[0].id);
		}
	}, [nodes, activeNodeId]);

	const totalInfluencePower = nodes.reduce((acc, n) => acc + Math.abs(n.weight), 0);
	const netSystemBalance = nodes.reduce((acc, n) => acc + n.weight, 0);
	const meanConfidence =
		nodes.length === 0 ? 0 : nodes.reduce((acc, n) => acc + (n.confidence ?? 0), 0) / nodes.length;

	const gridColumns = 24;
	const gridRows = 16;
	const vectorArrows = useMemo(() => {
		const arrows: Array<{ x: number; y: number; dx: number; dy: number; mag: number }> = [];
		if (nodes.length === 0) return arrows;

		for (let c = 1; c < gridColumns; c++) {
			for (let r = 1; r < gridRows; r++) {
				const vx = (dimensions.width / gridColumns) * c;
				const vy = (dimensions.height / gridRows) * r;

				let fx = 0;
				let fy = 0;

				for (const node of nodes) {
					const dx = vx - node.x;
					const dy = vy - node.y;
					const distSq = dx * dx + dy * dy + 400;
					const magnitude = (node.weight * 240000) / distSq;

					fx += (dx / Math.sqrt(distSq)) * magnitude;
					fy += (dy / Math.sqrt(distSq)) * magnitude;
				}

				const noiseIntensity = (1 - meanConfidence) * 25;
				if (noiseIntensity > 0) {
					const noiseX = Math.sin(vx * 0.05 + vy * 0.02 + phase) * noiseIntensity;
					const noiseY = Math.cos(vy * 0.05 + vx * 0.02 + phase) * noiseIntensity;
					fx += noiseX;
					fy += noiseY;
				}

				const totalMag = Math.sqrt(fx * fx + fy * fy) || 0.001;
				const arrowLength = Math.min(18, Math.max(4, totalMag * 0.12));

				arrows.push({
					x: vx,
					y: vy,
					dx: (fx / totalMag) * arrowLength,
					dy: (fy / totalMag) * arrowLength,
					mag: totalMag,
				});
			}
		}

		return arrows;
	}, [nodes, dimensions.width, dimensions.height, meanConfidence, phase]);

	const contourLevels = [25, 55, 95];
	const activeNode = nodes.find((n) => n.id === activeNodeId) ?? nodes[0] ?? null;
	const cx = dimensions.width / 2;
	const cy = dimensions.height / 2;

	if (nodes.length === 0) {
		return (
			<div
				className="flex h-full w-full flex-col overflow-hidden bg-(--sunken) text-(--f1)"
				ref={containerRef}
			>
				<Toolbar>
					<Icon name="spark" size="m" className="text-(--f3)" />
					<Typography.Label size="m" tone="f3" className="mr-1 shrink-0">
						Influence field
					</Typography.Label>
					<Chip label="nodes" value={0} />
					<Chip label="edges" value={0} />
				</Toolbar>
				<div className="flex flex-1 items-center justify-center px-8 text-center font-mono text-[12px] text-(--f4)">
					Loading static lineage report…
				</div>
			</div>
		);
	}

	return (
		<div
			className="flex h-full w-full flex-col overflow-hidden bg-(--sunken) text-(--f1)"
			ref={containerRef}
		>
			<Toolbar>
				<Icon name="spark" size="m" className="text-(--f3)" />
				<Typography.Label size="m" tone="f3" className="mr-1 shrink-0">
					Influence field
				</Typography.Label>
				<Chip label="nodes" value={nodes.length} />
				<Chip label="edges" value={edges.length} />
				<button
					type="button"
					onClick={() => setIsPlaying((v) => !v)}
					className="ml-auto rounded-[3px] border border-(--line) bg-(--raised) px-2 py-1 font-mono text-[10px] text-(--f3) hover:text-(--f1)"
				>
					{isPlaying ? "pause field" : "resume field"}
				</button>
			</Toolbar>

			<div className="relative min-h-0 flex-1">
				{activeNode ? (
					<div
						key={activeNode.id}
						style={{
							left: `${(activeNode.x / dimensions.width) * 100}%`,
							top: `calc(${(activeNode.y / dimensions.height) * 100}% - 36px)`,
							transform: "translateX(-50%)",
						}}
						className="pointer-events-none absolute flex items-center gap-2 whitespace-nowrap rounded-md border border-(--line) bg-(--surface)/90 px-2.5 py-1 font-mono text-[10px] font-semibold text-(--f1) backdrop-blur-md"
					>
						<span
							className="h-2 w-2 rounded-full"
							style={{
								backgroundColor: activeNode.weight >= 0 ? "var(--up)" : "var(--down)",
							}}
						/>
						{shortLabel(activeNode)}
						<span className="text-[9px] text-(--f4) tabular-nums">({activeNode.status})</span>
					</div>
				) : null}

				<svg
					role="img"
					aria-label="Metric usage field"
					viewBox={`0 0 ${dimensions.width} ${dimensions.height}`}
					className="block h-full w-full"
				>
					{/* Background Grid */}
					<g opacity="0.08" stroke="currentColor" strokeWidth="1" className="text-(--f4)">
						{Array.from({ length: gridRows }).map((_, i) => {
							const y = (dimensions.height / gridRows) * i;
							return <line key={`hg-${y}`} x1="0" y1={y} x2={dimensions.width} y2={y} />;
						})}
						{Array.from({ length: gridColumns }).map((_, i) => {
							const x = (dimensions.width / gridColumns) * i;
							return <line key={`vg-${x}`} x1={x} y1="0" x2={x} y2={dimensions.height} />;
						})}
					</g>

					{/* Contour Rings — radius from the node's own derived weight */}
					{nodes.map((node) => (
						<g key={`contour-${node.id}`} opacity={activeNodeId === node.id ? 0.25 : 0.05}>
							{contourLevels.map((level, idx) => {
								const computedRadius = Math.max(10, Math.abs(node.weight) * (idx + 1) * 15);
								return (
									<circle
										key={`c-ring-${level}`}
										cx={node.x}
										cy={node.y}
										r={computedRadius}
										fill="none"
										stroke={node.weight >= 0 ? "var(--up)" : "var(--down)"}
										strokeWidth="1.5"
										strokeDasharray={node.weight < 0 ? "4 4" : "none"}
									/>
								);
							})}
						</g>
					))}

					{/* Vector Field Arrows */}
					<g opacity="0.65">
						{vectorArrows.map((arrow) => {
							const strokeOp = Math.min(0.8, Math.max(0.1, arrow.mag * 0.04));
							return (
								<g
									key={`arrow-${arrow.x}-${arrow.y}`}
									transform={`translate(${arrow.x}, ${arrow.y})`}
									opacity={strokeOp}
								>
									<line
										x1={-arrow.dx / 2}
										y1={-arrow.dy / 2}
										x2={arrow.dx / 2}
										y2={arrow.dy / 2}
										stroke="var(--f3)"
										strokeWidth="1.5"
										strokeLinecap="round"
									/>
									<circle cx={arrow.dx / 2} cy={arrow.dy / 2} r="1.5" fill="var(--f2)" />
								</g>
							);
						})}
					</g>

					{/* Connective Chords — one per shared-consumer edge */}
					<g opacity="0.7">
						{edges.map((edge) => {
							const source = nodes.find((n) => n.id === edge.from);
							const target = nodes.find((n) => n.id === edge.to);
							if (!source || !target) return null;

							const midX = (source.x + target.x) / 2;
							const midY = (source.y + target.y) / 2;
							const weightFactor = 0.18;
							const ctrlX = midX + (cx - midX) * weightFactor;
							const ctrlY = midY + (cy - midY) * weightFactor;

							const isActive = activeNodeId === source.id || activeNodeId === target.id;
							const confidence = edge.confidence ?? 0;
							const strokeWidth = Math.min(4, 0.6 + Math.abs(edge.weight ?? 0) * 3);

							return (
								<path
									key={`chord-${edge.from}-${edge.to}`}
									d={`M ${source.x} ${source.y} Q ${ctrlX} ${ctrlY} ${target.x} ${target.y}`}
									fill="none"
									stroke={isActive ? "var(--f2)" : "var(--f4)"}
									strokeWidth={isActive ? strokeWidth + 1 : strokeWidth}
									strokeOpacity={Math.max(0.15, confidence)}
									className="transition-colors duration-300"
								>
									<title>
										{edge.from} → {edge.to} · {edge.reason ?? edge.relation ?? "shared consumer"}
									</title>
								</path>
							);
						})}
					</g>

					{/* Node Hubs */}
					{nodes.map((node) => {
						const isSelected = node.id === activeNodeId;
						const color = node.weight >= 0 ? "var(--up)" : "var(--down)";
						return (
							// biome-ignore lint/a11y/useSemanticElements: an SVG <g> holding <circle> children can't become a <button>.
							<g
								key={`hub-${node.id}`}
								transform={`translate(${node.x}, ${node.y})`}
								className="cursor-pointer"
								role="button"
								tabIndex={0}
								aria-label={`Select ${shortLabel(node)}`}
								aria-pressed={isSelected}
								onClick={() => setActiveNodeId(node.id)}
								onKeyDown={(event) => {
									if (event.key === "Enter" || event.key === " ") {
										event.preventDefault();
										setActiveNodeId(node.id);
									}
								}}
							>
								<title>{shortLabel(node)}</title>
								<circle
									r={Math.max(16, 16 + Math.abs(node.weight) * 10)}
									fill={color}
									opacity={isSelected ? 0.2 + Math.sin(phase * 2) * 0.1 : 0.1}
									className="transition-opacity duration-200"
								/>
								<circle
									r={isSelected ? 11 : 8}
									fill={color}
									stroke="var(--sunken)"
									strokeWidth="2"
									className="transition-all duration-300"
								/>
								{isSelected && (
									<circle
										r="16"
										fill="none"
										stroke={color}
										strokeWidth="1.5"
										strokeDasharray="4 3"
									/>
								)}
							</g>
						);
					})}
				</svg>
			</div>

			{/* Bottom Panel — real field summary + selected node detail */}
			<div className="border-(--line) border-t bg-(--surface)/90 backdrop-blur-xl">
				<div className="flex items-center justify-center gap-8 border-(--line) border-b px-6 py-3 md:gap-16">
					<div className="flex flex-col items-center gap-1">
						<span className="text-[9px] font-semibold uppercase tracking-wider text-(--f4)">
							Total Influence
						</span>
						<span className="text-sm font-bold tabular-nums text-(--f1)">
							{totalInfluencePower.toFixed(2)}
						</span>
					</div>
					<div className="flex flex-col items-center gap-1">
						<span className="text-[9px] font-semibold uppercase tracking-wider text-(--f4)">
							System Balance
						</span>
						<span className="text-sm font-bold tabular-nums text-(--f1)">
							{netSystemBalance > 0 ? "+" : ""}
							{netSystemBalance.toFixed(2)}
						</span>
					</div>
					<div className="flex flex-col items-center gap-1">
						<span className="text-[9px] font-semibold uppercase tracking-wider text-(--f4)">
							Used Share
						</span>
						<span className="text-sm font-bold tabular-nums text-(--warn)">
							{(meanConfidence * 100).toFixed(1)}%
						</span>
					</div>
				</div>

				{activeNode ? (
					<div className="flex flex-col gap-3 p-4">
						<Flex.Row className="items-center justify-between gap-2">
							<span className="truncate font-mono text-[12px] font-semibold text-(--f1)">
								{activeNode.id}
							</span>
							<span
								className={cn(
									"font-mono text-[10px] tabular-nums",
									activeNode.weight >= 0 ? "text-(--up)" : "text-(--down)",
								)}
							>
								{activeNode.status}
							</span>
						</Flex.Row>
						<div className="grid grid-cols-2 gap-x-6 gap-y-1 font-mono text-[10px] text-(--f4) md:grid-cols-4">
							<span>
								source <b className="font-normal text-(--f2)">{activeNode.source ?? "—"}</b>
							</span>
							<span>
								declared{" "}
								<b className="font-normal text-(--f2)">
									{activeNode.producer
										? `${activeNode.producer.file}:${activeNode.producer.line}`
										: "—"}
								</b>
							</span>
							<span>
								consumers{" "}
								<b className="font-normal text-(--f2)">
									{activeNode.producer?.consumers.length ?? 0}
								</b>
							</span>
							<span>
								edges <b className="font-normal text-(--f2)">{activeNode.edgeCount}</b>
							</span>
						</div>
					</div>
				) : null}
			</div>
		</div>
	);
};
