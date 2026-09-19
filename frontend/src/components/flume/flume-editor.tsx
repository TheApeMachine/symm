"use client";

import {
	DownloadIcon,
	GitCommitVerticalIcon,
	RefreshCwIcon,
	SplineIcon,
	UploadIcon,
	WaypointsIcon,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import { usePipelineGraphRow } from "#/collections/pipeline_graph_row";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Modal } from "#/components/ui/modal";
import { Textarea } from "#/components/ui/textarea";
import { toastManager } from "#/components/ui/toast";
import { ToggleGroup, ToggleGroupItem } from "#/components/ui/toggle-group";
import { Typography } from "#/components/ui/typography";
import { useDefinitions, usePrimitives } from "#/service/compute";
import {
	buildFlumeConfigFromSchemas,
	ensureNodeType,
} from "./build-config-from-schemas";
import type { EdgeRoutingMode } from "./connectionCalculator";
import { setRoutingMode, useRoutingMode } from "./flume-editor.store";
import {
	type BackendGraph,
	fetchAndImportDefinition,
	importJSONGraphToCollection,
} from "./import-graph";
import { NodeEditor, type NodeEditorHandle } from "./NodeEditor";

const LOCAL_GRAPH_ID = "local-default";

const ROUTING_OPTIONS: ReadonlyArray<{
	value: EdgeRoutingMode;
	label: string;
	icon: typeof SplineIcon;
	hint: string;
}> = [
	{ value: "smooth", label: "Smooth", icon: SplineIcon, hint: "Curved bezier" },
	{
		value: "straight",
		label: "Straight",
		icon: GitCommitVerticalIcon,
		hint: "Direct line",
	},
	{
		value: "orthogonal",
		label: "Orthogonal",
		icon: WaypointsIcon,
		hint: "Right-angle, A*-routed",
	},
];

const EdgeRoutingToggle = ({
	value,
	onChange,
}: {
	value: EdgeRoutingMode;
	onChange: (next: EdgeRoutingMode) => void;
}) => {
	return (
		<Flex.Row className="items-center gap-2">
			<Typography.Label size="s" tone="f4">
				Edges
			</Typography.Label>
			<ToggleGroup
				name="flume-edge-routing"
				onValueChange={onChange}
				value={value}
			>
				{ROUTING_OPTIONS.map((option) => {
					const Icon = option.icon;

					return (
						<ToggleGroupItem
							aria-label={option.hint}
							key={option.value}
							title={option.hint}
							value={option.value}
						>
							<Icon className="size-3.5" />
							<span className="hidden sm:inline">{option.label}</span>
						</ToggleGroupItem>
					);
				})}
			</ToggleGroup>
		</Flex.Row>
	);
};

type FlumeEditorProps = {
	projectId?: string;
};

export const FlumeEditor = ({ projectId }: FlumeEditorProps) => {
	const { data: operations, isPending, isError, isSuccess } = usePrimitives();
	const { data: definitions } = useDefinitions();

	const [selectedGraph, setSelectedGraph] = useState<string>("system");
	const [importModalOpen, setImportModalOpen] = useState(false);
	const [pastedJSON, setPastedJSON] = useState("");

	const graphId = projectId ?? LOCAL_GRAPH_ID;
	const routingMode = useRoutingMode();
	const editorHandleRef = useRef<NodeEditorHandle | null>(null);

	const row = usePipelineGraphRow(graphId);

	// Build FlumeConfig from backend operation schemas and ensure all nodes in current row exist
	const flumeConfig = useMemo(() => {
		const config = buildFlumeConfigFromSchemas(operations ?? {});
		if (row?.nodes) {
			for (const node of Object.values(row.nodes)) {
				if (node && typeof node === "object" && "type" in node) {
					ensureNodeType(config, String((node as { type: string }).type));
				}
			}
		}
		return config;
	}, [operations, row?.nodes]);

	const editorMode = isError || !isSuccess ? "builtin-only" : "full";

	/*
		Auto-import compiled architecture on initial visit:
		If the canvas has no nodes or is empty, automatically load the master system graph.
	*/
	useEffect(() => {
		if (isPending) {
			return;
		}

		const existing = pipelineGraphCollection.get(graphId);
		const isEmpty =
			!existing ||
			!existing.nodes ||
			Object.keys(existing.nodes).length === 0;

		if (isEmpty) {
			fetchAndImportDefinition(selectedGraph, graphId, projectId ?? null).catch(
				(err) => {
					console.error("Auto-import failed for architecture:", err);
				},
			);
		}
	}, [isPending, graphId, selectedGraph, projectId]);

	const handleSwitchDefinition = async (nextDef: string) => {
		setSelectedGraph(nextDef);
		try {
			await fetchAndImportDefinition(nextDef, graphId, projectId ?? null);
			toastManager.add({
				title: `Loaded ${nextDef}`,
				description: `Imported ${nextDef} architecture into canvas`,
				type: "success",
				timeout: 3000,
			});
		} catch (e) {
			toastManager.add({
				title: "Failed to load definition",
				description: e instanceof Error ? e.message : String(e),
				type: "error",
				timeout: 5000,
			});
		}
	};

	const handleReloadDefinition = async () => {
		try {
			await fetchAndImportDefinition(selectedGraph, graphId, projectId ?? null);
			toastManager.add({
				title: "Architecture Reloaded",
				description: `Re-imported and cleanly laid out ${selectedGraph}`,
				type: "success",
				timeout: 3000,
			});
		} catch (e) {
			toastManager.add({
				title: "Reload failed",
				description: e instanceof Error ? e.message : String(e),
				type: "error",
				timeout: 5000,
			});
		}
	};

	const handleImportPastedJSON = () => {
		try {
			const parsed = JSON.parse(pastedJSON) as BackendGraph;
			if (!parsed || typeof parsed !== "object" || !parsed.nodes) {
				throw new Error("Invalid graph format: missing 'nodes' object");
			}

			importJSONGraphToCollection(parsed, graphId, projectId ?? null);
			setImportModalOpen(false);
			setPastedJSON("");
			toastManager.add({
				title: "Graph Imported",
				description: `Successfully imported ${Object.keys(parsed.nodes).length} nodes`,
				type: "success",
				timeout: 3000,
			});
		} catch (e) {
			toastManager.add({
				title: "JSON Parse Error",
				description: e instanceof Error ? e.message : String(e),
				type: "error",
				timeout: 5000,
			});
		}
	};

	const handleExportJSON = () => {
		const currentRow = pipelineGraphCollection.get(graphId);
		if (!currentRow || !currentRow.nodes) {
			toastManager.add({
				title: "Canvas Empty",
				description: "No nodes available to export",
				type: "warning",
				timeout: 3000,
			});
			return;
		}

		const exportableGraph = {
			id: selectedGraph,
			name: selectedGraph,
			nodes: currentRow.nodes,
		};

		const formatted = JSON.stringify(exportableGraph, null, 2);
		navigator.clipboard.writeText(formatted).then(
			() => {
				toastManager.add({
					title: "Exported to Clipboard",
					description: "Graph definition copied as JSON",
					type: "success",
					timeout: 3000,
				});
			},
			(err) => {
				toastManager.add({
					title: "Clipboard Write Failed",
					description: String(err),
					type: "error",
					timeout: 5000,
				});
			},
		);
	};

	if (isPending) {
		return (
			<div className="flex min-h-[75vh] flex-1 items-center justify-center text-(--f3) text-sm">
				Loading operation schemas and architecture…
			</div>
		);
	}

	const allDefinitions = Array.from(
		new Set(["system", "logic", "execution", ...(definitions ?? [])]),
	);

	return (
		<div className="flex min-h-[75vh] flex-1 flex-col gap-3">
			<Flex.Row className="shrink-0 items-center justify-between gap-3 rounded-[4px] border bg-(--raised)/48 px-3 py-2">
				<Flex.Row className="items-center gap-3">
					<div className="flex items-center gap-2">
						<Typography.Label size="s" tone="f4">
							Architecture
						</Typography.Label>
						<select
							className="h-7 rounded border border-(--line2) bg-(--sunken) px-2 font-mono text-xs text-(--f1) outline-none focus:border-(--accent)"
							onChange={(e) => handleSwitchDefinition(e.target.value)}
							value={selectedGraph}
						>
							<optgroup label="Orchestration & Stages">
								<option value="system">system (Master Orchestration)</option>
								<option value="logic">logic (Cognition & Attractors)</option>
								<option value="execution">execution (Policy & Gating)</option>
							</optgroup>
							<optgroup label="Signal Extraction Graphs">
								{allDefinitions
									.filter(
										(d) =>
											d !== "system" &&
											d !== "logic" &&
											d !== "execution",
									)
									.sort()
									.map((d) => (
										<option key={d} value={d}>
											{d}
										</option>
									))}
							</optgroup>
						</select>
					</div>

					<Button
						onClick={handleReloadDefinition}
						size="s"
						title="Re-import clean compiled architecture definition from backend"
						type="button"
						variant="quiet"
					>
						<RefreshCwIcon className="size-3.5" />
						Reload Architecture
					</Button>

					<Button
						onClick={() => setImportModalOpen(true)}
						size="s"
						title="Import any raw JSON graph definition"
						type="button"
						variant="quiet"
					>
						<UploadIcon className="size-3.5" />
						Import JSON
					</Button>

					<Button
						onClick={handleExportJSON}
						size="s"
						title="Copy current graph JSON to clipboard"
						type="button"
						variant="quiet"
					>
						<DownloadIcon className="size-3.5" />
						Export JSON
					</Button>

					<Typography.Span className="text-xs" variant="muted">
						{Object.keys(flumeConfig.nodeTypes).length} node types
					</Typography.Span>
				</Flex.Row>

				<EdgeRoutingToggle onChange={setRoutingMode} value={routingMode} />
			</Flex.Row>

			<NodeEditor
				key={`${editorMode}:${graphId}`}
				className="min-h-0 flex-1"
				edgeRoutingMode={routingMode}
				graphId={graphId}
				nodeTypes={flumeConfig.nodeTypes}
				portTypes={flumeConfig.portTypes}
				projectId={projectId ?? null}
				ref={editorHandleRef}
				style={{ minHeight: isError ? "70vh" : "75vh" }}
			/>

			{importModalOpen && (
				<Modal
					open={importModalOpen}
					onClose={() => setImportModalOpen(false)}
					size="lg"
				>
					<Modal.Header>
						<span className="font-mono text-sm font-semibold text-(--f1)">
							Import Graph JSON
						</span>
						<Modal.Close onClick={() => setImportModalOpen(false)} />
					</Modal.Header>
					<Modal.Body className="flex flex-col gap-2">
						<p className="text-xs text-(--f3)">
							Paste any declarative JSON graph definition below. Nodes and
							connections will be automatically arranged by dependency rank.
						</p>
						<Textarea
							className="h-64 font-mono text-xs"
							onChange={(e) => setPastedJSON(e.target.value)}
							placeholder={`{\n  "nodes": {\n    "source": { "type": "data.Source", ... },\n    "sink": { "type": "data.Sink", ... }\n  }\n}`}
							value={pastedJSON}
						/>
					</Modal.Body>
					<Modal.Footer>
						<Button
							onClick={() => setImportModalOpen(false)}
							variant="outline"
						>
							Cancel
						</Button>
						<Button
							disabled={!pastedJSON.trim()}
							onClick={handleImportPastedJSON}
							variant="solid"
						>
							Import to Canvas
						</Button>
					</Modal.Footer>
				</Modal>
			)}
		</div>
	);
};
