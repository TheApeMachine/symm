import { useCallback, useEffect, useMemo, useState } from "react";
import { ModelScope } from "#/components/graph/component";
import type { Graph as RenderGraph } from "#/components/graph/core/graph";
import {
	applyPendingGraphSurface,
	type MarketGraphEdge,
	type MarketGraphNode,
	paintGraphSurface,
	readGraphSurface,
	subscribeGraphSurface,
} from "#/components/terminal/graph-surface-store";
import { graphStore } from "#/collections/app";
import { Panel } from "#/components/ui/panel";
import { Button } from "@/components/ui/button";

import { Chip } from "@/components/ui/chip";
import { Flex } from "@/components/ui/flex";
import { Input } from "@/components/ui/input";
import { List } from "@/components/ui/list";
import { Section } from "@/components/ui/section";
import { Toolbar } from "@/components/ui/toolbar";
import { Typography } from "@/components/ui/typography";

const searchNodes = (
	graph: RenderGraph | undefined,
	query: string,
): string[] => {
	if (!graph || query.trim() === "") {
		return [];
	}

	const needle = query.trim().toLowerCase();

	return Object.keys(graph.nodes)
		.filter((name) => name.toLowerCase().includes(needle))
		.sort((left, right) => {
			const leftExact = left.toLowerCase() === needle ? 1 : 0;
			const rightExact = right.toLowerCase() === needle ? 1 : 0;

			if (leftExact !== rightExact) {
				return rightExact - leftExact;
			}

			return left.localeCompare(right);
		})
		.slice(0, 16);
};

const GraphSearch = ({
	graph,
	onPick,
}: {
	graph: RenderGraph | undefined;
	onPick: (name: string) => void;
}) => {
	const [query, setQuery] = useState("");
	const [open, setOpen] = useState(false);
	const matches = useMemo(() => searchNodes(graph, query), [graph, query]);

	const choose = (name: string) => {
		onPick(name);
		setQuery("");
		setOpen(false);
	};

	return (
		<div className="relative w-full max-w-sm">
			<Input.Search
				mono
				placeholder="Search nodes…"
				value={query}
				onChange={(event) => {
					setQuery(event.target.value);
					setOpen(true);
				}}
				onFocus={() => setOpen(true)}
				onKeyDown={(event) => {
					if (event.key === "Enter" && matches[0]) {
						event.preventDefault();
						choose(matches[0]);
					}

					if (event.key === "Escape") {
						setOpen(false);
						setQuery("");
					}
				}}
			/>

			{open && query.length > 0 ? (
				<List className="absolute top-full left-0 z-20 mt-1 max-h-72 w-full gap-0 overflow-auto rounded-md border border-(--line) bg-(--surface) py-1 shadow-[0_18px_40px_-18px_rgba(0,0,0,0.78)]">
					{matches.length === 0 ? (
						<List.Empty className="px-3 py-2 text-left text-[11px]">
							No matches.
						</List.Empty>
					) : (
						matches.map((name) => (
							<List.Option
								key={name}
								size="s"
								className="rounded-none"
								onClick={() => choose(name)}
								label={
									<span className="font-mono text-[11px]">
										{name.split(".").at(-1) ?? name}
									</span>
								}
								trailing={
									<span className="truncate font-mono text-[10px] text-(--f4)">
										{name}
									</span>
								}
							/>
						))
					)}
				</List>
			) : null}
		</div>
	);
};

const SelectedNodePanel = ({
	edges,
	node,
	nodeName,
}: {
	edges: MarketGraphEdge[];
	node: MarketGraphNode | null;
	nodeName: string | null;
}) => {
	if (node === null || nodeName === null) {
		return (
			<Panel variant="raised" size="m" className="gap-2">
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Node detail
				</div>
				<div className="font-mono text-[11px] text-(--f4)">
					Select a node in the graph to inspect its value, confidence, and
					connected edges.
				</div>
			</Panel>
		);
	}

	const related = edges.filter(
		(edge) => edge.from === nodeName || edge.to === nodeName,
	);
	const metadataEntries = Object.entries(node.metadata ?? {});

	return (
		<Flex.Column gap={2}>
			<Panel variant="raised" size="m" className="gap-2">
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Node detail
				</div>
				<div className="font-semibold text-[14px] text-(--f1)">{nodeName}</div>
				<div className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono text-[11px]">
					<span className="text-(--f4)">kind</span>
					<span>{node.kind ?? "—"}</span>
					<span className="text-(--f4)">source</span>
					<span>{node.source ?? "—"}</span>
					<span className="text-(--f4)">symbol</span>
					<span>{node.symbol ?? "—"}</span>
					<span className="text-(--f4)">value</span>
					<span>
						{typeof node.value === "number" ? node.value.toFixed(6) : "—"}
					</span>
					<span className="text-(--f4)">strength</span>
					<span>
						{typeof node.strength === "number" ? node.strength.toFixed(6) : "—"}
					</span>
					<span className="text-(--f4)">confidence</span>
					<span>
						{typeof node.confidence === "number"
							? node.confidence.toFixed(6)
							: "—"}
					</span>
					<span className="text-(--f4)">at</span>
					<span>{node.at ?? "—"}</span>
				</div>
			</Panel>

			<Panel variant="raised" size="m" className="gap-2">
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Metadata
				</div>
				{metadataEntries.length === 0 ? (
					<div className="font-mono text-[11px] text-(--f4)">
						No metadata on this node.
					</div>
				) : (
					<div className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono text-[11px]">
						{metadataEntries.map(([key, value]) => (
							<Flex key={key} className="contents">
								<span className="text-(--f4)">{key}</span>
								<span className="break-all">
									{typeof value === "string" ? value : JSON.stringify(value)}
								</span>
							</Flex>
						))}
					</div>
				)}
			</Panel>

			<Panel variant="raised" size="m" className="gap-2">
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Connected edges
				</div>
				{related.length === 0 ? (
					<div className="font-mono text-[11px] text-(--f4)">
						No connected edges.
					</div>
				) : (
					<div className="flex max-h-80 flex-col gap-1 overflow-auto font-mono text-[11px]">
						{related.map((edge) => (
							<div
								key={`${edge.from}:${edge.to}:${edge.relation ?? "edge"}:${edge.at ?? ""}:${edge.reason ?? ""}`}
								className="rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5"
							>
								<div className="text-(--f2)">
									{edge.from} → {edge.to}
								</div>
								<div className="mt-0.5 text-(--f4)">
									{edge.relation ?? "relationless"} · w=
									{typeof edge.weight === "number"
										? edge.weight.toFixed(6)
										: "—"}{" "}
									· c=
									{typeof edge.confidence === "number"
										? edge.confidence.toFixed(6)
										: "—"}
								</div>
								{edge.reason ? (
									<div className="mt-0.5 text-(--f4)">{edge.reason}</div>
								) : null}
							</div>
						))}
					</div>
				)}
			</Panel>
		</Flex.Column>
	);
};

export const GraphSurface = () => {
	const [, setVersion] = useState(0);
	const [selectedNodeName, setSelectedNodeName] = useState<string | null>(null);

	useEffect(() => {
		const notify = () => setVersion((value) => value + 1);
		const unsubscribe = subscribeGraphSurface(notify);
		const unregister = graphStore.subscribe((value) => {
			paintGraphSurface(value);
		});
		paintGraphSurface(graphStore.state);
		notify();

		return () => {
			unsubscribe();
			unregister.unsubscribe();
		};
	}, []);

	const snapshot = readGraphSurface();
	const frame = snapshot.frame;
	const graph = snapshot.graph ?? undefined;
	const edges = frame?.edges;
	const topologyPending = snapshot.topologyPending;
	const handleNodeSelect = useCallback(
		(_index: number, nodeName: string) => setSelectedNodeName(nodeName),
		[],
	);

	useEffect(() => {
		if (
			selectedNodeName !== null &&
			graph?.nodes[selectedNodeName] === undefined
		) {
			setSelectedNodeName(null);
		}
	}, [graph, selectedNodeName]);

	// retainedFrame lives outside React so the painter can accept WebSocket
	// frames without remounting ModelScope; its edge-array identity is the memo key.
	const relationCounts = useMemo(() => {
		const counts = new Map<string, number>();

		for (const edge of edges ?? []) {
			const relation = edge.relation ?? "unknown";
			counts.set(relation, (counts.get(relation) ?? 0) + 1);
		}

		return [...counts.entries()].sort((left, right) => right[1] - left[1]);
	}, [edges]);

	const selectedNode =
		selectedNodeName === null
			? null
			: (frame?.nodes?.[selectedNodeName] ?? null);

	return (
		<div className="flex h-full min-w-295 flex-col">
			<Toolbar>
				<Typography.Label size="m" tone="f3" className="mr-1 shrink-0">
					Market graph
				</Typography.Label>
				<Chip label="nodes" value={Object.keys(frame?.nodes ?? {}).length} />
				<Chip label="edges" value={(frame?.edges ?? []).length} />
				<Chip label="relations" value={relationCounts.length} />
				<Chip label="at" value={frame?.at ?? "—"} />
				{topologyPending ? (
					<Button
						variant="solid"
						tone="warning"
						size="xs"
						className="ml-auto"
						onClick={applyPendingGraphSurface}
					>
						Sync topology
					</Button>
				) : (
					<span className="ml-auto font-mono text-[9px] text-(--f4)">
						live inspection · stable topology
					</span>
				)}
			</Toolbar>

			<div className="grid min-h-0 flex-1 grid-cols-[minmax(760px,1fr)_372px]">
				<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--sunken)">
					<Section.Header className="gap-3">
						<GraphSearch graph={graph} onPick={setSelectedNodeName} />
						<Flex.Row wrap="wrap" gap={1} className="ml-auto gap-1.5">
							{relationCounts.slice(0, 4).map(([relation, count]) => (
								<Chip
									key={relation}
									variant="quiet"
									size="xs"
									label={relation}
									value={count}
								/>
							))}
						</Flex.Row>
					</Section.Header>

					<div className="min-h-0 flex-1">
						{graph ? (
							<ModelScope
								className="h-full"
								graph={graph}
								onNodeSelect={handleNodeSelect}
								selectedNodeName={selectedNodeName}
							/>
						) : (
							<div className="flex h-full items-center justify-center px-8 text-center font-mono text-[12px] text-(--f4)">
								Waiting for a live market graph frame.
							</div>
						)}
					</div>
				</div>

				<div className="min-h-0 overflow-auto bg-(--surface) p-3.5">
					<SelectedNodePanel
						edges={edges ?? []}
						node={selectedNode}
						nodeName={selectedNodeName}
					/>
				</div>
			</div>
		</div>
	);
};