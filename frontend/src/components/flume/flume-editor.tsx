"use client";

import {
	AlertTriangleIcon,
	CheckCircleIcon,
	CpuIcon,
	DownloadIcon,
	GitCommitVerticalIcon,
	LayoutGridIcon,
	PlayIcon,
	PlusIcon,
	RefreshCwIcon,
	SaveIcon,
	SplineIcon,
	UploadIcon,
	WaypointsIcon,
	XIcon,
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
import { hubBaseUrl } from "#/lib/hub";
import { useDefinitions } from "#/service/compute";
import type { EdgeRoutingMode } from "./connectionCalculator";
import type { CompilerDiagnostic, NodeLogEntry, NodeStatus } from "./context";
import { createFlumeConfig } from "./flume-config.generated";
import { setRoutingMode, useRoutingMode } from "./flume-editor.store";
import {
	clearGraphResults,
	setGraphResults,
	useGraphResults,
} from "./graph-results.store";
import {
	type BackendGraph,
	fetchAndImportDefinition,
	importJSONGraphToCollection,
	reconcileDefinition,
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
	const { data: definitions, refetch: refetchDefinitions } = useDefinitions();

	const [selectedGraph, setSelectedGraph] = useState<string>("system");
	const [importModalOpen, setImportModalOpen] = useState(false);
	const [newDefModalOpen, setNewDefModalOpen] = useState(false);
	const [newDefName, setNewDefName] = useState("");
	const [pastedJSON, setPastedJSON] = useState("");

	const [diagnostics, setDiagnostics] = useState<CompilerDiagnostic[]>([]);
	const [statuses, setStatuses] = useState<Record<string, NodeStatus>>({});
	const [logs, setLogs] = useState<Record<string, NodeLogEntry[]>>({});
	const [isSaving, setIsSaving] = useState(false);
	const [isCompiling, setIsCompiling] = useState(false);
	const [isRunning, setIsRunning] = useState(false);

	const graphId = projectId ?? LOCAL_GRAPH_ID;
	const row = usePipelineGraphRow(graphId);
	const routingMode = useRoutingMode();
	const editorHandleRef = useRef<NodeEditorHandle | null>(null);

	const graphVersion = JSON.stringify(row?.nodes ?? {});
	const activeResults = useGraphResults(graphId, graphVersion);

	const allDefinitions = useMemo(() => {
		return Array.from(
			new Set(["system", "logic", "execution", ...(definitions ?? [])]),
		);
	}, [definitions]);

	// Build authoritative FlumeConfig from generated primitive types and dynamic definitions
	const flumeConfig = useMemo(() => {
		return createFlumeConfig(allDefinitions);
	}, [allDefinitions]);

	/*
		Auto-import compiled architecture on initial visit:
		If the canvas has no nodes or is empty, automatically load the master system graph.
	*/
	useEffect(() => {
		reconcileDefinition(selectedGraph, graphId, projectId ?? null)
			.then((replaced: boolean) => {
				if (!replaced) {
					return;
				}

				toastManager.add({
					title: `Reloaded ${selectedGraph}`,
					description: "The definition changed since this was drawn",
					type: "info",
					timeout: 4000,
				});
			})
			.catch((err: unknown) => {
				console.error("Auto-import failed for architecture:", err);
			});
	}, [graphId, selectedGraph, projectId]);

	const handleSwitchDefinition = async (nextDef: string) => {
		setSelectedGraph(nextDef);
		setDiagnostics([]);
		clearGraphResults(graphId);
		setStatuses({});
		setLogs({});
		try {
			await fetchAndImportDefinition(nextDef, graphId, projectId ?? null);
			toastManager.add({
				title: `Loaded ${nextDef}`,
				description: `Imported ${nextDef} architecture into canvas`,
				type: "success",
				timeout: 3000,
			});
		} catch (err: unknown) {
			toastManager.add({
				title: "Failed to load definition",
				description: err instanceof Error ? err.message : String(err),
				type: "error",
				timeout: 5000,
			});
		}
	};

	const handleReloadDefinition = async () => {
		setDiagnostics([]);
		clearGraphResults(graphId);
		try {
			await fetchAndImportDefinition(selectedGraph, graphId, projectId ?? null);
			toastManager.add({
				title: "Architecture Reloaded",
				description: `Re-imported and cleanly laid out ${selectedGraph}`,
				type: "success",
				timeout: 3000,
			});
		} catch (err: unknown) {
			toastManager.add({
				title: "Reload failed",
				description: err instanceof Error ? err.message : String(err),
				type: "error",
				timeout: 5000,
			});
		}
	};

	const handleAutoLayout = () => {
		if (!editorHandleRef.current?.hasNodes()) {
			toastManager.add({
				title: "No Nodes to Layout",
				description: "The canvas is currently empty",
				type: "info",
				timeout: 3000,
			});
			return;
		}

		editorHandleRef.current.autoLayout("orthogonal");
		toastManager.add({
			title: "Auto Layout Applied",
			description: "Nodes positioned for optimal orthogonal routing",
			type: "success",
			timeout: 3000,
		});
	};

	const handleSaveDefinition = async () => {
		const currentRow = pipelineGraphCollection.get(graphId);
		const nodes = currentRow?.nodes ?? {};

		const payload = {
			id: selectedGraph,
			name: selectedGraph,
			nodes,
		};

		setIsSaving(true);
		try {
			const res = await fetch(
				`${hubBaseUrl()}/workbench/signals/${selectedGraph}`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(payload),
				},
			);
			if (!res.ok) {
				const errText = await res.text();
				throw new Error(errText || res.statusText);
			}

			// Also update local storage collection row for the definition
			pipelineGraphCollection.insert({
				id: selectedGraph,
				project_id: projectId ?? null,
				schema_version: 1,
				nodes,
				comments: {},
				viewport: { scale: 1, translate: { x: 0, y: 0 } },
				updated_at: new Date(),
			});

			refetchDefinitions();
			toastManager.add({
				title: "Saved Definition",
				description: `Definition ${selectedGraph} saved to backend (in-process)`,
				type: "success",
				timeout: 3000,
			});
		} catch (e) {
			toastManager.add({
				title: "Save Failed",
				description: e instanceof Error ? e.message : String(e),
				type: "error",
				timeout: 5000,
			});
		} finally {
			setIsSaving(false);
		}
	};

	const handleCompile = async () => {
		const currentRow = pipelineGraphCollection.get(graphId);
		const nodes = currentRow?.nodes ?? {};

		const payload = {
			id: selectedGraph,
			nodes,
		};

		setIsCompiling(true);
		try {
			const res = await fetch(`${hubBaseUrl()}/workbench/compile`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(payload),
			});
			const data = await res.json();
			if (data.ok) {
				setDiagnostics([]);
				toastManager.add({
					title: "Compilation Succeeded",
					description: `Valid Cap'n Proto program (${data.nodeCount} nodes, ${data.routeCount} routes)`,
					type: "success",
					timeout: 4000,
				});
			} else {
				const diags = data.diagnostics || [
					{ kind: "compile_error", message: data.error },
				];
				setDiagnostics(diags);
				toastManager.add({
					title: "Compilation Failed",
					description: data.error || "Compiler reported diagnostics",
					type: "error",
					timeout: 6000,
				});
			}
		} catch (e) {
			toastManager.add({
				title: "Compile Request Failed",
				description: e instanceof Error ? e.message : String(e),
				type: "error",
				timeout: 5000,
			});
		} finally {
			setIsCompiling(false);
		}
	};

	const handleRun = async () => {
		const currentRow = pipelineGraphCollection.get(graphId);
		const nodes = currentRow?.nodes ?? {};

		const payload = {
			id: selectedGraph,
			nodes,
		};

		const version = JSON.stringify(nodes);
		clearGraphResults(graphId);
		setIsRunning(true);
		try {
			const res = await fetch(`${hubBaseUrl()}/workbench/run`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(payload),
			});
			const data = await res.json();
			if (data.ok) {
				setDiagnostics([]);
				const runResults = data.results || {};
				setGraphResults(graphId, version, runResults);
				if (data.statuses) {
					setStatuses(data.statuses);
				}
				if (data.logs) {
					setLogs(data.logs);
				}
				toastManager.add({
					title: "Execution Succeeded",
					description:
						"Evaluated 1 observation through real Cap'n Proto program",
					type: "success",
					timeout: 4000,
				});
			} else {
				if (data.statuses) {
					setStatuses(data.statuses);
				}
				if (data.logs) {
					setLogs(data.logs);
				}
				if (data.diagnostics && data.diagnostics.length > 0) {
					setDiagnostics(data.diagnostics);
				}
				toastManager.add({
					title: "Execution Failed",
					description: data.error || "Program execution failed",
					type: "error",
					timeout: 6000,
				});
			}
		} catch (err: unknown) {
			toastManager.add({
				title: "Run Request Failed",
				description: err instanceof Error ? err.message : String(err),
				type: "error",
				timeout: 5000,
			});
		} finally {
			setIsRunning(false);
		}
	};

	const handleCreateNewDefinition = () => {
		const cleanName = newDefName
			.trim()
			.toLowerCase()
			.replace(/[^a-z0-9_-]/g, "_");
		if (!cleanName) return;

		setSelectedGraph(cleanName);
		setDiagnostics([]);
		clearGraphResults(graphId);

		// Initialize empty graph row in collection
		const emptyNodes = {};
		pipelineGraphCollection.insert({
			id: graphId,
			project_id: projectId ?? null,
			schema_version: 1,
			nodes: emptyNodes,
			comments: {},
			viewport: { scale: 1, translate: { x: 0, y: 0 } },
			updated_at: new Date(),
		});

		// Also update current draft in collection
		pipelineGraphCollection.update(graphId, (draft) => {
			draft.nodes = emptyNodes;
			draft.updated_at = new Date();
		});

		setNewDefModalOpen(false);
		setNewDefName("");
		toastManager.add({
			title: "Created Definition",
			description: `Working on new definition: ${cleanName}`,
			type: "success",
			timeout: 3000,
		});
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

	return (
		<Flex.Column gap={2} className="min-h-[75vh] flex-1">
			{/* Top action toolbar */}
			<Flex.Row
				align="center"
				justify="between"
				gap={3}
				className="shrink-0 flex-wrap rounded-sm border bg-(--raised)/48 px-3 py-2"
			>
				<Flex.Row align="center" gap={2} className="flex-wrap">
					<Flex.Row align="center" gap={2}>
						<Typography.Label size="s" tone="f4">
							Definition
						</Typography.Label>
						<select
							className="h-7 rounded border border-(--line2) bg-(--sunken) px-2 font-mono text-xs text-(--f1) outline-none focus:border-(--accent)"
							onChange={(e) => handleSwitchDefinition(e.target.value)}
							value={selectedGraph}
							data-testid="definition-select"
						>
							<optgroup label="Architecture">
								<option value="system">system</option>
								<option value="logic">logic</option>
								<option value="execution">execution</option>
							</optgroup>
							<optgroup label="Definitions">
								{allDefinitions
									.filter(
										(d) => d !== "system" && d !== "logic" && d !== "execution",
									)
									.sort()
									.map((d) => (
										<option key={d} value={d}>
											{d}
										</option>
									))}
							</optgroup>
						</select>

						<Button
							onClick={() => setNewDefModalOpen(true)}
							size="s"
							title="Create a new definition"
							type="button"
							variant="outline"
							data-testid="new-definition-button"
						>
							<PlusIcon className="size-3.5" />
							New
						</Button>
					</Flex.Row>

					<div className="h-4 w-px bg-(--line)/40" />

					{/* Workbench Primary Execution Actions */}
					<Button
						onClick={handleSaveDefinition}
						disabled={isSaving}
						size="s"
						title="Save graph definition to backend"
						type="button"
						variant="solid"
						className="bg-blue-600 hover:bg-blue-500 text-white font-medium"
						data-testid="save-button"
					>
						<SaveIcon className="size-3.5" />
						{isSaving ? "Saving…" : "Save"}
					</Button>

					<Button
						onClick={handleCompile}
						disabled={isCompiling}
						size="s"
						title="Compile with real Cap'n Proto compiler"
						type="button"
						variant="outline"
						className="border-purple-500/50 text-purple-300 hover:bg-purple-950/40"
						data-testid="compile-button"
					>
						<CpuIcon className="size-3.5" />
						{isCompiling ? "Compiling…" : "Compile"}
					</Button>

					<Button
						onClick={handleRun}
						disabled={isRunning}
						size="s"
						title="Compile and evaluate 1 observation"
						type="button"
						variant="solid"
						className="bg-emerald-600 hover:bg-emerald-500 text-white font-medium"
						data-testid="run-button"
					>
						<PlayIcon className="size-3.5" />
						{isRunning ? "Running…" : "Run"}
					</Button>

					<div className="h-4 w-px bg-(--line)/40" />

					<Button
						onClick={handleReloadDefinition}
						size="s"
						title="Re-import clean compiled architecture definition from backend"
						type="button"
						variant="quiet"
					>
						<RefreshCwIcon className="size-3.5" />
						Reload
					</Button>

					<Button
						onClick={handleAutoLayout}
						size="s"
						title="Auto-layout nodes for clean orthogonal routing"
						type="button"
						variant="quiet"
					>
						<LayoutGridIcon className="size-3.5" />
						Auto Layout
					</Button>

					<Button
						onClick={() => setImportModalOpen(true)}
						size="s"
						title="Import raw JSON"
						type="button"
						variant="quiet"
					>
						<UploadIcon className="size-3.5" />
						Import
					</Button>

					<Button
						onClick={handleExportJSON}
						size="s"
						title="Export JSON to clipboard"
						type="button"
						variant="quiet"
					>
						<DownloadIcon className="size-3.5" />
						Export
					</Button>
				</Flex.Row>

				<EdgeRoutingToggle onChange={setRoutingMode} value={routingMode} />
			</Flex.Row>

			{/* Diagnostics Banner */}
			{diagnostics.length > 0 && (
				<Flex.Row
					align="center"
					justify="between"
					className="rounded border border-red-500/50 bg-red-950/70 px-3 py-2 text-xs text-red-200"
					data-testid="diagnostics-banner"
				>
					<Flex.Row align="center" gap={2}>
						<AlertTriangleIcon className="size-4 shrink-0 text-red-400" />
						<div>
							<span className="font-semibold uppercase tracking-wider text-red-400">
								Compiler Diagnostic [{diagnostics[0].kind}]:
							</span>{" "}
							<span className="font-mono">{diagnostics[0].message}</span>
							{diagnostics[0].edgeFrom && (
								<span className="ml-2 font-mono text-red-300">
									({diagnostics[0].edgeFrom} &rarr; {diagnostics[0].edgeTo})
								</span>
							)}
						</div>
					</Flex.Row>
					<button
						type="button"
						onClick={() => setDiagnostics([])}
						className="text-red-400 hover:text-red-200"
					>
						<XIcon className="size-4" />
					</button>
				</Flex.Row>
			)}

			{/* Execution Result Banner */}
			{Object.keys(activeResults).length > 0 && (
				<Flex.Row
					align="center"
					justify="between"
					className="rounded border border-emerald-500/50 bg-emerald-950/70 px-3 py-2 text-xs text-emerald-200"
					data-testid="results-banner"
				>
					<Flex.Row align="center" gap={2}>
						<CheckCircleIcon className="size-4 shrink-0 text-emerald-400" />
						<div>
							<span className="font-semibold uppercase tracking-wider text-emerald-400">
								Program Output:
							</span>{" "}
							<span className="font-mono">
								{Object.entries(activeResults)
									.map(
										([nodeId, portOutputs]) =>
											`${nodeId}: ${JSON.stringify(portOutputs)}`,
									)
									.join(" | ")}
							</span>
						</div>
					</Flex.Row>
					<button
						type="button"
						onClick={() => {
							clearGraphResults(graphId);
						}}
						className="text-emerald-400 hover:text-emerald-200"
					>
						<XIcon className="size-4" />
					</button>
				</Flex.Row>
			)}

			{/* Main Canvas Editor */}
			<NodeEditor
				key={graphId}
				className="min-h-0 flex-1"
				edgeRoutingMode={routingMode}
				graphId={graphId}
				nodeTypes={flumeConfig.nodeTypes}
				portTypes={flumeConfig.portTypes}
				projectId={projectId ?? null}
				ref={editorHandleRef}
				diagnostics={diagnostics}
				results={activeResults}
				statuses={statuses}
				logs={logs}
				style={{ minHeight: "75vh" }}
			/>

			{/* New Definition Modal */}
			{newDefModalOpen && (
				<Modal
					open={newDefModalOpen}
					onClose={() => setNewDefModalOpen(false)}
					size="s"
				>
					<Modal.Header>
						<span className="font-mono text-sm font-semibold text-(--f1)">
							Create New Definition
						</span>
						<Modal.Close onClick={() => setNewDefModalOpen(false)} />
					</Modal.Header>
					<Modal.Body className="flex flex-col gap-3">
						<p className="text-xs text-(--f3)">
							Name your reusable graph definition. It can be composed as a node
							inside other graphs.
						</p>
						<input
							type="text"
							className="h-8 w-full rounded border border-(--line2) bg-(--sunken) px-3 font-mono text-xs text-(--f1) outline-none focus:border-(--accent)"
							placeholder="e.g. inner, custom_signal, my_pipeline"
							value={newDefName}
							onChange={(event) => setNewDefName(event.target.value)}
							onKeyDown={(event) => {
								if (event.key === "Enter") handleCreateNewDefinition();
							}}
							data-testid="new-definition-input"
						/>
					</Modal.Body>
					<Modal.Footer>
						<Button onClick={() => setNewDefModalOpen(false)} variant="outline">
							Cancel
						</Button>
						<Button
							disabled={!newDefName.trim()}
							onClick={handleCreateNewDefinition}
							variant="solid"
							data-testid="create-definition-submit"
						>
							Create
						</Button>
					</Modal.Footer>
				</Modal>
			)}

			{/* Import Modal */}
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
							Paste any declarative JSON graph definition below.
						</p>
						<Textarea
							className="h-64 font-mono text-xs"
							onChange={(event) => setPastedJSON(event.target.value)}
							placeholder={`{\n  "nodes": {\n    "source": { "type": "data.Source", ... },\n    "sink": { "type": "data.Sink", ... }\n  }\n}`}
							value={pastedJSON}
						/>
					</Modal.Body>
					<Modal.Footer>
						<Button onClick={() => setImportModalOpen(false)} variant="outline">
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
		</Flex.Column>
	);
};
