"use client";

import { useAuth } from "@clerk/tanstack-react-start";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangleIcon,
	ChevronRightIcon,
	HomeIcon,
	RefreshCwIcon,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import {
	Select,
	SelectItem,
	SelectPopup,
	SelectTrigger,
	SelectValue,
} from "#/components/ui/select";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import {
	backendAuthHeaders,
	backendBaseURL,
	type ClerkGetToken,
} from "#/lib/backend-http";
import { AttentionPanel } from "./attention-panel";
import { ModelScope } from "./component";
import { Graph } from "./core/graph";
import { buildSubgraph } from "./graph-helpers";
import { LogitLensPanel } from "./logit-lens-panel";
import { NodeInspectorPanel } from "./node-inspector-panel";
import { NodeSearchBar } from "./search-bar";
import { WeightsPanel } from "./weights-panel";

const useModelList = (getToken: ClerkGetToken) => {
	return useQuery<string[]>({
		queryKey: ["modelscope"],
		queryFn: async () => {
			const headers = await backendAuthHeaders(getToken);
			const response = await fetch(`${backendBaseURL()}/backend/modelscope`, {
				headers,
			});

			if (!response.ok) {
				const text = await response.text();
				throw new Error(
					`GET /backend/modelscope failed (${response.status}): ${text || response.statusText}`,
				);
			}

			const body = (await response.json()) as unknown;

			if (!Array.isArray(body)) {
				throw new Error(
					`Expected an array of model names, got ${typeof body}: ${JSON.stringify(body).slice(0, 120)}`,
				);
			}

			return body.filter((value): value is string => typeof value === "string");
		},
		retry: 1,
		staleTime: 60_000,
	});
};

const useInspectModel = (name: string, getToken: ClerkGetToken) => {
	return useQuery({
		queryKey: ["modelscope", name],
		queryFn: async () => {
			const headers = await backendAuthHeaders(getToken);
			const inspectURL = `${backendBaseURL()}/backend/modelscope/inspect?path=${encodeURIComponent(`models/${name}`)}`;
			const response = await fetch(inspectURL, { headers });
			return response.json();
		},
		enabled: Boolean(name),
	});
};

const estimateLayerCount = (graph: Graph | undefined): number => {
	if (!graph) return 12;
	let max = -1;
	for (const name of Object.keys(graph.nodes)) {
		const match = name.match(/(?:blk|layers?|h)\.(\d+)/);
		if (match) {
			const index = Number.parseInt(match[1] ?? "", 10);
			if (Number.isFinite(index) && index > max) max = index;
		}
	}
	return max < 0 ? 12 : max + 1;
};

const DrillBreadcrumb = ({
	root,
	onPick,
}: {
	root: string;
	onPick: (next: string) => void;
}) => {
	if (root === "" || root === "__model__") {
		return (
			<Flex.Row className="items-center gap-1 px-3 py-1.5">
				<HomeIcon
					aria-hidden
					className="size-3.5 shrink-0 text-muted-foreground"
				/>
				<Typography.Span className="text-xs" variant="muted">
					Full model
				</Typography.Span>
			</Flex.Row>
		);
	}

	const parts = root.split(".");
	const ancestors = [
		{ name: "__model__", label: "model" },
		...parts.map((_, index) => ({
			name: parts.slice(0, index + 1).join("."),
			label: parts[index],
		})),
	];

	return (
		<Flex.Row className="flex-wrap items-center gap-1 px-3 py-1.5">
			<HomeIcon
				aria-hidden
				className="size-3.5 shrink-0 text-muted-foreground"
			/>
			{ancestors.map((entry, index) => (
				<Flex.Row className="items-center gap-1" key={entry.name}>
					<button
						className="rounded px-1 py-0.5 font-mono text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground"
						onClick={() => onPick(entry.name)}
						type="button"
					>
						{entry.label}
					</button>
					{index < ancestors.length - 1 ? (
						<ChevronRightIcon
							aria-hidden
							className="size-3 text-muted-foreground/60"
						/>
					) : null}
				</Flex.Row>
			))}
		</Flex.Row>
	);
};

const ToolbarRow = ({
	selected,
	onSelect,
	modelNames,
	modelsLoading,
	modelsError,
	onRefreshModels,
	inspectLoading,
	inspectError,
	stats,
	graph,
	onSearchPick,
}: {
	selected: string;
	onSelect: (next: string) => void;
	modelNames: ReadonlyArray<string>;
	modelsLoading: boolean;
	modelsError: Error | null;
	onRefreshModels: () => void;
	inspectLoading: boolean;
	inspectError: Error | null;
	stats: { nodes: number; edges: number } | null;
	graph: Graph | undefined;
	onSearchPick: (name: string) => void;
}) => {
	const noModels = !modelsLoading && !modelsError && modelNames.length === 0;

	return (
		<Flex.Column className="shrink-0 gap-1.5 rounded-xl border bg-muted/48 px-3 py-2">
			<Flex.Row align="center" gap={3}>
				<Typography.Span className="whitespace-nowrap text-xs" variant="muted">
					Model
				</Typography.Span>
				<Select
					disabled={modelsLoading || modelNames.length === 0}
					onValueChange={(value) => {
						if (value) onSelect(value);
					}}
					value={selected}
				>
					<SelectTrigger className="min-w-72" size="sm">
						<SelectValue
							placeholder={
								modelsLoading
									? "Loading models…"
									: modelsError
										? "Failed to load models"
										: noModels
											? "No model files found in ./models/"
											: "Select a model…"
							}
						/>
					</SelectTrigger>
					<SelectPopup>
						{modelNames.map((name) => (
							<SelectItem key={name} value={name}>
								{name}
							</SelectItem>
						))}
					</SelectPopup>
				</Select>

				<Button
					aria-label="Refresh model list"
					disabled={modelsLoading}
					onClick={onRefreshModels}
					size="icon-sm"
					type="button"
					variant="ghost"
				>
					<RefreshCwIcon className={modelsLoading ? "animate-spin" : ""} />
				</Button>

				<Badge
					size="sm"
					variant={modelNames.length > 0 ? "outline" : "warning"}
				>
					{modelNames.length} available
				</Badge>

				<NodeSearchBar graph={graph} onPick={onSearchPick} />

				{inspectLoading ? (
					<Typography.Span className="text-xs" variant="muted">
						Parsing model…
					</Typography.Span>
				) : null}

				{stats ? (
					<Flex.Row className="ml-auto items-center gap-2">
						<Badge size="sm" variant="outline">
							{stats.nodes.toLocaleString()} nodes
						</Badge>
						<Badge size="sm" variant="outline">
							{stats.edges.toLocaleString()} edges
						</Badge>
					</Flex.Row>
				) : null}
			</Flex.Row>

			{modelsError ? (
				<Flex.Row className="items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5">
					<AlertTriangleIcon
						aria-hidden
						className="mt-0.5 size-3.5 shrink-0 text-destructive"
					/>
					<Typography.Span className="text-[11px]" variant="error">
						{modelsError.message}
					</Typography.Span>
				</Flex.Row>
			) : null}

			{inspectError ? (
				<Flex.Row className="items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5">
					<AlertTriangleIcon
						aria-hidden
						className="mt-0.5 size-3.5 shrink-0 text-destructive"
					/>
					<Typography.Span className="text-[11px]" variant="error">
						{inspectError.message}
					</Typography.Span>
				</Flex.Row>
			) : null}
		</Flex.Column>
	);
};

type SidePanel = "node" | "weights" | "attention" | "logitlens";

const isGroupName = (graph: Graph | undefined, name: string): boolean => {
	if (!graph) return false;
	const prefix = `${name}.`;
	for (const candidate of Object.keys(graph.nodes)) {
		if (candidate !== name && candidate.startsWith(prefix)) return true;
	}
	return false;
};

export const ModelScopeInspector = () => {
	const [mounted, setMounted] = useState(false);
	const [selected, setSelected] = useState("");
	const [selectedNode, setSelectedNode] = useState<string | null>(null);
	const [subgraphRoot, setSubgraphRoot] = useState<string>("__model__");
	const [sidePanel, setSidePanel] = useState<SidePanel>("node");
	const { getToken } = useAuth();
	const queryClient = useQueryClient();
	const {
		data: modelNames = [],
		isLoading: modelsLoading,
		error: modelsError,
	} = useModelList(getToken);
	const {
		data: graphData,
		isLoading: inspectLoading,
		error: inspectError,
	} = useInspectModel(selected, getToken);

	useEffect(() => {
		setMounted(true);
	}, []);

	const fullGraph = useMemo(() => {
		if (!graphData) return undefined;
		const next = new Graph();
		next.loadFromData(graphData);
		return next;
	}, [graphData]);

	const visibleGraph = useMemo(() => {
		if (!fullGraph) return undefined;
		if (subgraphRoot === "" || subgraphRoot === "__model__") return fullGraph;
		return buildSubgraph(fullGraph, subgraphRoot);
	}, [fullGraph, subgraphRoot]);

	const layerCount = useMemo(() => estimateLayerCount(fullGraph), [fullGraph]);

	const stats = visibleGraph
		? {
				nodes: Object.keys(visibleGraph.nodes).length,
				edges: Object.keys(visibleGraph.edges).length,
			}
		: null;

	const drillInto = (name: string) => {
		if (!fullGraph) return;
		setSubgraphRoot(name);
		setSelectedNode(name);
	};

	const handleSelect = (name: string | null) => {
		setSelectedNode(name);
		if (name && fullGraph && isGroupName(fullGraph, name)) {
			// Switch to node tab so the user sees structure + can drill.
			setSidePanel("node");
		}
	};

	if (!mounted) return null;

	return (
		<Flex.Column fullWidth fullHeight gap={2}>
			<ToolbarRow
				graph={fullGraph}
				inspectError={(inspectError as Error | null) ?? null}
				inspectLoading={inspectLoading}
				modelNames={modelNames}
				modelsError={(modelsError as Error | null) ?? null}
				modelsLoading={modelsLoading}
				onRefreshModels={() => {
					void queryClient.invalidateQueries({ queryKey: ["modelscope"] });
				}}
				onSearchPick={(name) => setSelectedNode(name)}
				onSelect={(name) => {
					setSelected(name);
					setSelectedNode(null);
					setSubgraphRoot("__model__");
				}}
				selected={selected}
				stats={stats}
			/>

			<div className="grid min-h-0 flex-1 grid-cols-1 gap-2 lg:grid-cols-[minmax(0,1fr)_400px]">
				<Flex.Column
					className="min-h-0 overflow-hidden rounded-xl border bg-card/20"
					fullHeight
					fullWidth
				>
					<Flex.Row className="items-center justify-between border-b">
						<DrillBreadcrumb onPick={setSubgraphRoot} root={subgraphRoot} />
						{subgraphRoot !== "__model__" && subgraphRoot !== "" ? (
							<Button
								className="mr-2"
								onClick={() => setSubgraphRoot("__model__")}
								size="xs"
								variant="ghost"
							>
								Reset
							</Button>
						) : null}
					</Flex.Row>
					<Flex.Column className="min-h-0 flex-1" fullWidth>
						<ModelScope
							graph={visibleGraph}
							onNodeSelect={(_, name) => handleSelect(name)}
							selectedNodeName={selectedNode}
						/>
					</Flex.Column>
				</Flex.Column>

				<Flex.Column
					className="min-h-0 overflow-hidden rounded-xl border bg-card/40"
					fullHeight
				>
					<Tabs
						className="flex min-h-0 flex-1 flex-col"
						onValueChange={(value) => {
							if (
								value === "node" ||
								value === "weights" ||
								value === "attention" ||
								value === "logitlens"
							) {
								setSidePanel(value);
							}
						}}
						value={sidePanel}
					>
						<Tabs.List className="shrink-0 border-b px-2">
							<Tabs.Tab value="node">Node</Tabs.Tab>
							<Tabs.Tab value="weights">Weights</Tabs.Tab>
							<Tabs.Tab value="attention">Attention</Tabs.Tab>
							<Tabs.Tab value="logitlens">Logit Lens</Tabs.Tab>
						</Tabs.List>
						<Tabs.Panel className="min-h-0 flex-1 overflow-auto" value="node">
							<NodeInspectorPanel
								graph={fullGraph}
								onDrillInto={drillInto}
								onSelect={setSelectedNode}
								selectedName={selectedNode}
							/>
						</Tabs.Panel>
						<Tabs.Panel
							className="min-h-0 flex-1 overflow-auto"
							value="weights"
						>
							<WeightsPanel graph={fullGraph} selectedName={selectedNode} />
						</Tabs.Panel>
						<Tabs.Panel
							className="min-h-0 flex-1 overflow-auto"
							value="attention"
						>
							<AttentionPanel layerCount={layerCount} />
						</Tabs.Panel>
						<Tabs.Panel
							className="min-h-0 flex-1 overflow-auto"
							value="logitlens"
						>
							<LogitLensPanel layerCount={layerCount} />
						</Tabs.Panel>
					</Tabs>
				</Flex.Column>
			</div>
		</Flex.Column>
	);
};
