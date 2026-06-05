import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";

/*
Decision Tree — the live playbook. The story instruments every evaluation and
publishes a `decision_tree` frame: the playbook flattened into nodes, each with
how often it was reached and how often its condition held. Rendered as a tree, it
shows where evaluations travel and — more usefully — where they die: a node that
is reached constantly but almost never holds is the bottleneck starving the desk
of trades.
*/

type ConditionFrame = {
	label: string;
	negated: boolean;
	held: number;
};

type TreeNodeFrame = {
	key: string;
	depth: number;
	parent: string;
	label: string;
	action: string;
	reached: number;
	held: number;
	combinator: string;
	conditions: ConditionFrame[];
};

type RecentDecision = { symbol: string; action: string; ts: string };

type DecisionFrame = {
	evaluations: number;
	nodes: TreeNodeFrame[];
	recent: RecentDecision[];
};

const socketUrl =
	(import.meta.env.VITE_SYMM_WS_URL as string | undefined)?.trim() ||
	"ws://127.0.0.1:8765/ws";

const NODE_W = 210;
const NODE_H = 78;
const ROW_H = 132;
const PAD = 24;

const pct = (value: number) => `${Math.round(value * 100)}%`;

const DecisionsPage = () => {
	const [frame, setFrame] = useState<DecisionFrame | null>(null);
	const [selectedKey, setSelectedKey] = useState<string | null>(null);

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				if (raw.chart === "decision_tree") {
					setFrame(raw as unknown as DecisionFrame);
				}
			} catch {
				return;
			}
		},
	});

	const layout = useMemo(() => {
		if (!frame || frame.nodes.length === 0) {
			return null;
		}

		const byDepth: TreeNodeFrame[][] = [];

		for (const node of frame.nodes) {
			(byDepth[node.depth] ??= []).push(node);
		}

		const widest = Math.max(...byDepth.map((row) => (row ? row.length : 0)));
		const width = widest * NODE_W + PAD * 2;
		const height = byDepth.length * ROW_H + PAD * 2;

		const pos = new Map<string, { x: number; y: number }>();

		byDepth.forEach((row, depth) => {
			if (!row) {
				return;
			}

			const rowWidth = row.length * NODE_W;
			const startX = (width - rowWidth) / 2 + NODE_W / 2;

			row.forEach((node, index) => {
				pos.set(node.key, {
					x: startX + index * NODE_W,
					y: PAD + depth * ROW_H + NODE_H / 2,
				});
			});
		});

		return { width, height, pos };
	}, [frame]);

	const evaluations = frame?.evaluations ?? 0;
	const recent = frame?.recent ?? [];

	// Default the breakdown to the worst bottleneck: a compound node reached often
	// but whose condition rarely holds — the one starving the desk of trades.
	const bottleneckKey = useMemo(() => {
		if (!frame) {
			return null;
		}

		let best: { key: string; rate: number; reached: number } | null = null;

		for (const node of frame.nodes) {
			if (node.reached === 0 || (node.conditions?.length ?? 0) < 2) {
				continue;
			}

			const rate = node.held / node.reached;

			if (
				!best ||
				rate < best.rate ||
				(rate === best.rate && node.reached > best.reached)
			) {
				best = { key: node.key, rate, reached: node.reached };
			}
		}

		return best?.key ?? null;
	}, [frame]);

	const activeKey = selectedKey ?? bottleneckKey;
	const selectedNode =
		frame?.nodes.find((node) => node.key === activeKey) ?? null;

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-3">
			<div className="flex flex-wrap items-baseline justify-between gap-2">
				<div className="flex flex-col">
					<h1 className="text-lg font-semibold">Decision Tree</h1>
					<p className="text-xs text-muted-foreground">
						The playbook, live — where evaluations travel and where they die.
					</p>
				</div>
				<div className="text-xs text-muted-foreground">
					{evaluations.toLocaleString()} evaluations ·{" "}
					<span className="text-emerald-400">holds</span> /{" "}
					<span className="text-rose-400">bottleneck</span> /{" "}
					<span className="text-zinc-500">never reached</span>
				</div>
			</div>

			<div className="flex min-h-0 flex-1 gap-3">
				<div className="min-h-0 flex-1 overflow-auto rounded-xl border border-border bg-card/40 p-2">
					{!frame || !layout ? (
						<div className="p-6 text-sm text-muted-foreground">
							Waiting for the story to publish a decision tree… (needs a loaded
							playbook and live measurements)
						</div>
					) : (
						<svg
							viewBox={`0 0 ${layout.width} ${layout.height}`}
							className="h-full w-full"
							role="img"
							aria-label="decision tree"
						>
							{frame.nodes.map((node) => {
								if (!node.parent) {
									return null;
								}

								const child = layout.pos.get(node.key);
								const parent = layout.pos.get(node.parent);

								if (!child || !parent) {
									return null;
								}

								return (
									<line
										key={`edge-${node.key}`}
										x1={parent.x}
										y1={parent.y + NODE_H / 2}
										x2={child.x}
										y2={child.y - NODE_H / 2}
										stroke="currentColor"
										className="text-border"
										strokeWidth={1.5}
									/>
								);
							})}

							{frame.nodes.map((node) => {
								const point = layout.pos.get(node.key);

								if (!point) {
									return null;
								}

								const reachRate =
									evaluations > 0 ? node.reached / evaluations : 0;
								const holdRate =
									node.reached > 0 ? node.held / node.reached : 0;
								const dead = node.reached === 0;
								const bottleneck = !dead && holdRate < 0.02;

								const tone = dead
									? "border-zinc-700 bg-zinc-800/40 text-zinc-500"
									: bottleneck
										? "border-rose-500/50 bg-rose-500/10"
										: "border-emerald-500/40 bg-emerald-500/10";

								const selected = node.key === activeKey;

								return (
									<foreignObject
										key={`node-${node.key}`}
										x={point.x - NODE_W / 2 + 8}
										y={point.y - NODE_H / 2}
										width={NODE_W - 16}
										height={NODE_H}
									>
										<button
											type="button"
											onClick={() => setSelectedKey(node.key)}
											className={`flex h-full w-full flex-col justify-between rounded-lg border px-2 py-1.5 text-left ${tone} ${
												selected ? "ring-2 ring-sky-400" : ""
											}`}
										>
											<div className="flex items-start justify-between gap-1">
												<span className="line-clamp-2 font-mono text-[11px] leading-tight">
													{node.label}
												</span>
												{node.action ? (
													<span className="shrink-0 rounded bg-sky-500/20 px-1 text-[9px] text-sky-300">
														{node.action}
													</span>
												) : null}
											</div>
											<div className="flex items-center justify-between text-[9px] text-muted-foreground">
												<span>reach {pct(reachRate)}</span>
												<span>
													hold{" "}
													<span
														className={
															bottleneck ? "text-rose-400" : "text-emerald-400"
														}
													>
														{pct(holdRate)}
													</span>
												</span>
											</div>
										</button>
									</foreignObject>
								);
							})}
						</svg>
					)}
				</div>

				<div className="flex w-64 shrink-0 flex-col gap-3 overflow-auto">
					<div className="rounded-xl border border-border bg-card/40 p-2">
						<p className="px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
							Why it gates
						</p>
						{selectedNode ? (
							<div className="flex flex-col gap-1.5 p-1">
								<p className="font-mono text-[11px] leading-tight">
									{selectedNode.label}
								</p>
								<p className="text-[10px] text-muted-foreground">
									{selectedNode.combinator === "all"
										? "all must hold"
										: selectedNode.combinator === "any"
											? "any may hold"
											: "single condition"}{" "}
									· reached {selectedNode.reached.toLocaleString()}×
								</p>
								{(selectedNode.conditions ?? []).map((cond, index) => {
									const rate =
										selectedNode.reached > 0
											? cond.held / selectedNode.reached
											: 0;
									const failing = rate < 0.2;

									return (
										<div
											key={`${cond.label}-${index}`}
											className="flex flex-col gap-0.5"
										>
											<div className="flex items-center justify-between gap-2">
												<span className="truncate font-mono text-[10px]">
													{cond.negated ? "¬ " : ""}
													{cond.label}
												</span>
												<span
													className={`shrink-0 text-[10px] ${
														failing ? "text-rose-400" : "text-emerald-400"
													}`}
												>
													{pct(rate)}
												</span>
											</div>
											<div className="h-1.5 overflow-hidden rounded-full bg-muted">
												<div
													className={`h-full rounded-full ${
														failing ? "bg-rose-400" : "bg-emerald-400"
													}`}
													style={{ width: `${Math.round(rate * 100)}%` }}
												/>
											</div>
										</div>
									);
								})}
								{(selectedNode.conditions?.length ?? 0) === 0 ? (
									<p className="text-[10px] text-muted-foreground">
										Single leaf — its hold % is shown on the node.
									</p>
								) : null}
							</div>
						) : (
							<p className="px-1 text-xs text-muted-foreground">
								Click a node to see which of its conditions pass and which
								fail.
							</p>
						)}
					</div>

					<div className="flex flex-col gap-1 rounded-xl border border-border bg-card/40 p-2">
						<p className="px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
							Recent actions
						</p>
						{recent.length === 0 ? (
							<p className="px-1 text-xs text-muted-foreground">
								No actions fired yet.
							</p>
						) : (
							[...recent].reverse().map((decision, index) => (
								<div
									key={`${decision.ts}-${index}`}
									className="flex items-center justify-between rounded border border-border bg-card px-2 py-1 text-[11px]"
								>
									<span className="font-mono">{decision.action}</span>
									<span className="truncate text-muted-foreground">
										{decision.symbol}
									</span>
								</div>
							))
						)}
					</div>
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/decisions")({
	component: DecisionsPage,
});
