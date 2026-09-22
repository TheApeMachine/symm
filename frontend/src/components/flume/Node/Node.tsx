"use client";

import {
	Maximize2Icon,
	Minimize2Icon,
	NetworkIcon,
	TerminalIcon,
} from "lucide-react";
import type { RefObject } from "react";
import React from "react";
import { createPortal } from "react-dom";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import {
	ConnectionRecalculateContext,
	DiagnosticsContext,
	EditorIdContext,
	FlumeGraphWorkerContext,
	GraphIdContext,
	NodeActionsContext,
	type NodeLogEntry,
	NodeLogsContext,
	NodeResultsContext,
	type NodeStatus,
	NodeStatusesContext,
	NodeTypesContext,
	PortTypesContext,
	StageContext,
} from "#/components/flume/context";
import {
	setSelectedNode,
	useSelectedNode,
} from "#/components/flume/flume-editor.store";
import type {
	Connections,
	Coordinate,
	InputData,
	NodeHeaderRenderCallback,
	NodeMap,
	SelectOption,
} from "#/components/flume/types";
import { Badge } from "#/components/ui/badge";
import { Card, CardPanel } from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";
import { Form } from "#/components/ui/form";
import {
	Frame,
	FrameDescription,
	FrameHeader,
	FrameTitle,
} from "#/components/ui/frame";
import type { CompiledUINode } from "#/components/ui/renderer";
import { renderNode } from "#/components/ui/renderer";
import {
	type UIComponentName,
	uiComponents,
} from "#/components/ui/ui-component-registry.generated";
import { cn } from "@/lib/utils";
import ContextMenu from "../ContextMenu/ContextMenu";
import Draggable from "../Draggable/Draggable";
import IoPorts from "../IoPorts/IoPorts";
import { fetchAndImportDefinition } from "../import-graph";
import { NodeLogs } from "./NodeLogs";

class UIPreviewBoundary extends React.Component<
	{ children: React.ReactNode; fallback?: React.ReactNode },
	{ hasError: boolean }
> {
	constructor(props: {
		children: React.ReactNode;
		fallback?: React.ReactNode;
	}) {
		super(props);
		this.state = { hasError: false };
	}

	static getDerivedStateFromError() {
		return { hasError: true };
	}

	componentDidCatch(err: Error) {
		console.warn("UI Node preview error:", err);
	}

	render() {
		if (this.state.hasError) {
			return this.props.fallback ?? null;
		}

		return this.props.children;
	}
}

/* Lazy to avoid circular dep — NodeEditor imports Node */
const NodeEditor = React.lazy(() =>
	import("../NodeEditor").then((m) => ({ default: m.NodeEditor })),
);

const SUBGRAPH_WIDTH = 560;
const SUBGRAPH_HEIGHT = 360;

interface NodeProps {
	id: string;
	width: number;
	x: number;
	y: number;
	stageRect: RefObject<DOMRect | undefined>;
	connections: Connections;
	type: string;
	inputData: InputData;
	onDragStart: () => void;
	renderNodeHeader?: NodeHeaderRenderCallback;
	root?: boolean;
	subGraph?: NodeMap;
}

const Node = ({
	id,
	width,
	x,
	y,
	stageRect,
	connections,
	type,
	inputData,
	root,
	onDragStart,
	renderNodeHeader,
	subGraph,
}: NodeProps) => {
	const editorId = React.useContext(EditorIdContext);
	const selectedNodeId = useSelectedNode(editorId);
	const isSelected = selectedNodeId === id;

	const nodeTypes = React.useContext(NodeTypesContext) ?? {};
	const portTypes = React.useContext(PortTypesContext) ?? {};
	const nodeActions = React.useContext(NodeActionsContext);
	const graphWorker = React.useContext(FlumeGraphWorkerContext);
	const triggerRecalculation = React.useContext(ConnectionRecalculateContext);
	const stageState = React.useContext(StageContext) ?? {
		scale: 0,
		translate: { x: 0, y: 0 },
	};

	const currentNodeType = nodeTypes[type];
	const diagnostics = React.useContext(DiagnosticsContext) || [];
	const results = React.useContext(NodeResultsContext) || {};
	const statuses = React.useContext(NodeStatusesContext) || {};
	const allLogs = React.useContext(NodeLogsContext) || {};

	const nodeDiagnostics = diagnostics.filter(
		(d) => d.nodeId === id || (d.nodeType && d.nodeType === type),
	);
	const hasError = nodeDiagnostics.length > 0;
	const nodeResult = results[id];
	const nodeStatus: NodeStatus = statuses[id] ?? (hasError ? "error" : "init");

	const nodeLogs: NodeLogEntry[] = React.useMemo(() => {
		if (allLogs[id] && allLogs[id].length > 0) {
			return allLogs[id];
		}
		if (type && allLogs[type] && allLogs[type].length > 0) {
			return allLogs[type];
		}
		const normType = type ? type.toLowerCase() : "";
		const normParts = normType.split(".");
		for (const [key, entries] of Object.entries(allLogs)) {
			const normKey = key.toLowerCase();
			if (
				normKey === normType ||
				normType.includes(normKey) ||
				normKey.includes(normType)
			) {
				return entries;
			}
			const keyParts = normKey.split(".");
			if (
				normParts.length >= 2 &&
				keyParts.length >= 2 &&
				normParts[0] === keyParts[0]
			) {
				if (
					normParts[1].includes(keyParts[1]) ||
					keyParts[1].includes(normParts[1])
				) {
					return entries;
				}
			}
		}
		return [];
	}, [allLogs, id, type]);

	const [logsOpen, setLogsOpen] = React.useState(false);

	React.useEffect(() => {
		if (typeof logsOpen === "boolean") {
			triggerRecalculation?.();
		}
	}, [logsOpen, triggerRecalculation]);

	const statusMeta = React.useMemo(() => {
		switch (nodeStatus) {
			case "ready":
			case "ok":
				return { label: "READY", variant: "success" as const, pulse: false };
			case "busy":
				return { label: "BUSY", variant: "info" as const, pulse: true };
			case "waiting":
				return { label: "WAITING", variant: "warning" as const, pulse: false };
			case "error":
			case "fatal":
				return { label: "ERROR", variant: "error" as const, pulse: false };
			case "done":
				return { label: "DONE", variant: "brand" as const, pulse: false };
			default:
				return { label: "INIT", variant: "disabled" as const, pulse: false };
		}
	}, [nodeStatus]);

	const isDefinitionNode = Boolean(
		String(currentNodeType?.type ?? "").startsWith("definition:") ||
			currentNodeType?.category === "Definitions",
	);

	const isUiComponent =
		type.startsWith("ui.") &&
		type !== "ui.UIRoute" &&
		type.slice(3) in uiComponents;

	const compiledUiNode = React.useMemo(() => {
		if (!isUiComponent) {
			return null;
		}

		const compName = type.slice(3);
		const nodeProps: Record<string, unknown> = {};

		for (const [propName, rawEntry] of Object.entries(inputData ?? {})) {
			if (propName === "components" || propName.startsWith("components_")) {
				continue;
			}

			const extracted =
				rawEntry !== null && typeof rawEntry === "object"
					? "value" in rawEntry
						? (rawEntry as { value: unknown }).value
						: undefined
					: rawEntry;

			if (extracted !== undefined && extracted !== null && extracted !== "") {
				nodeProps[propName] = extracted;
			}
		}

		for (const [portName, targets] of Object.entries(
			connections?.inputs ?? {},
		)) {
			if (portName.startsWith("components")) {
				continue;
			}

			if (!targets || targets.length === 0) {
				continue;
			}

			const firstTarget = targets[0];

			nodeProps[portName] = {
				binding: {
					node: firstTarget.nodeId,
					port: firstTarget.portName ?? "out",
				},
			};
		}

		return {
			name: compName as UIComponentName,
			props: nodeProps,
		} as CompiledUINode;
	}, [isUiComponent, type, inputData, connections?.inputs]);

	const isBlock = Boolean(
		currentNodeType?.defaultSubGraph ||
			isDefinitionNode ||
			currentNodeType?.category === "memory" ||
			String(currentNodeType?.type ?? "").startsWith("block."),
	);

	const {
		label,
		deletable,
		description,
		inputs = [],
		outputs = [],
	} = currentNodeType ?? {
		label: "",
		inputs: [],
		outputs: [],
	};

	const nodeWrapper = React.useRef<HTMLDivElement>(null);
	const [menuOpen, setMenuOpen] = React.useState(false);
	const [menuCoordinates, setMenuCoordinates] = React.useState({ x: 0, y: 0 });
	const [subGraphOpen, setSubGraphOpen] = React.useState(false);
	const [subGraphFullscreen, setSubGraphFullscreen] = React.useState(false);

	const updateNodeConnections = () => {
		triggerRecalculation?.();
	};

	const stopDrag = (_event: unknown, coordinates: Coordinate) => {
		nodeActions?.setNodeCoordinates({
			nodeId: id,
			x: coordinates.x,
			y: coordinates.y,
		});
		graphWorker?.endDrag(id, coordinates.x, coordinates.y);
	};

	const handleDragStartWithGraph = () => {
		onDragStart();
		graphWorker?.beginDrag(id);
	};

	const handleDrag = ({ x, y }: Coordinate) => {
		if (!nodeWrapper.current) {
			return;
		}

		nodeWrapper.current.style.transform = `translate(${x}px,${y}px)`;
		graphWorker?.updateDrag(id, x, y);
	};

	const handleContextMenu = (event: MouseEvent | React.MouseEvent) => {
		event.preventDefault();
		event.stopPropagation();
		setSelectedNode(editorId, id);
		setMenuCoordinates({ x: event.clientX, y: event.clientY });
		setMenuOpen(true);
		return false;
	};

	const closeContextMenu = () => setMenuOpen(false);

	const deleteNode = () => {
		nodeActions?.removeNode(id);
	};

	const handleMenuOption = ({ value }: SelectOption) => {
		switch (value) {
			case "deleteNode":
				deleteNode();
				break;
			default:
				return;
		}
	};

	// Subgraph editors are full NodeEditor instances bound to their own
	// collection row at "${parentGraphId}:${nodeId}". No callback-driven
	// sync is needed — the sub-editor writes directly to its row and
	// useLiveQuery subscribers on either side re-render off the same
	// source of truth.
	const parentGraphId = React.useContext(GraphIdContext);
	const subgraphId = isDefinitionNode
		? currentNodeType.type
		: `${parentGraphId}:${id}`;

	const suppressEmbeddedPortControlPrep = React.useCallback(
		(event: React.MouseEvent<HTMLDivElement>) => {
			if (!(event.target instanceof Element)) return false;
			if (event.target.closest("button, input, textarea, select, option"))
				return true;
			// Suppress only when the click originates inside the nested sub-graph
			// editor, not the outer stage that the block node itself lives in.
			const subgraphContainer = event.currentTarget.querySelector(
				"[data-subgraph-editor]",
			);
			return Boolean(subgraphContainer?.contains(event.target));
		},
		[],
	);

	const portalContainer =
		typeof document !== "undefined" ? document.body : null;

	/*
		A definition node's operations live in its own collection row, keyed by
		the definition it is. The manifest behind it is held by the backend, so
		opening the node is what brings it down: without this the row stays
		empty and the sub-graph paints a blank canvas.
	*/
	React.useEffect(() => {
		if (!isDefinitionNode || !subGraphOpen) {
			return;
		}

		const existing = pipelineGraphCollection.get(subgraphId);

		if (existing && Object.keys(existing.nodes ?? {}).length > 0) {
			return;
		}

		const definition = String(currentNodeType?.type ?? "").slice(
			"definition:".length,
		);

		fetchAndImportDefinition(definition, subgraphId).catch(() => {
			// The node shows an empty canvas rather than tearing down the
			// stage it is drawn on; the definition select reports the failure.
		});
	}, [isDefinitionNode, subGraphOpen, subgraphId, currentNodeType?.type]);

	// A sub-graph shows its ports only while it is open.
	const showPorts = !isDefinitionNode || subGraphOpen;
	const prevShowPortsRef = React.useRef(showPorts);

	React.useLayoutEffect(() => {
		if (prevShowPortsRef.current && !showPorts) {
			graphWorker?.clearNodePortLayouts(id);

			if (nodeWrapper.current) {
				const clientWidth = nodeWrapper.current.clientWidth;
				const clientHeight = nodeWrapper.current.clientHeight;

				if (clientWidth > 0 && clientHeight > 0) {
					graphWorker?.setNodeLayout(id, clientWidth, clientHeight);
				}
			}

			triggerRecalculation?.();
		}

		if (!prevShowPortsRef.current && showPorts) {
			triggerRecalculation?.();
		}

		prevShowPortsRef.current = showPorts;
	}, [showPorts, id, graphWorker, triggerRecalculation]);

	const wiredPortCount =
		Object.keys(connections?.inputs ?? {}).length +
		Object.keys(connections?.outputs ?? {}).length;

	const resolvedSubGraph = isBlock ? (subGraph ?? true) : undefined;

	const subGraphEditor =
		resolvedSubGraph !== undefined && subGraphOpen ? (
			<React.Suspense fallback={null}>
				<NodeEditor
					graphId={subgraphId}
					nodeTypes={nodeTypes}
					portTypes={portTypes}
					disableComments
					disableFocusCapture
					className="rounded-[3px] border border-(--line)/48 bg-(--bg)/80"
					style={{
						width: SUBGRAPH_WIDTH,
						height: SUBGRAPH_HEIGHT,
						pointerEvents: "all",
					}}
				/>
			</React.Suspense>
		) : null;

	const fullscreenOverlay =
		resolvedSubGraph !== undefined &&
		subGraphOpen &&
		subGraphFullscreen &&
		portalContainer
			? createPortal(
					<Flex.Column className="fixed inset-0 z-50 bg-(--bg)">
						<Flex.Row
							align="center"
							gap={3}
							className="border-b px-4 py-2 text-sm text-(--f3)"
						>
							<NetworkIcon className="size-4" />
							<span className="font-medium text-(--f1)">{label}</span>
							<span className="flex-1">{description}</span>
							<button
								type="button"
								onClick={() => setSubGraphFullscreen(false)}
								className="ml-auto flex items-center gap-1.5 rounded px-2 py-1 hover:bg-(--raised)/60"
							>
								<Minimize2Icon className="size-4" />
								Exit full screen
							</button>
						</Flex.Row>
						<Flex.Row className="min-h-0 flex-1">
							<React.Suspense fallback={null}>
								<NodeEditor
									graphId={subgraphId}
									nodeTypes={nodeTypes}
									portTypes={portTypes}
									disableComments
									className="h-full w-full"
								/>
							</React.Suspense>
						</Flex.Row>
					</Flex.Column>,
					portalContainer,
				)
			: null;

	const nodeWidth =
		subGraphOpen && !subGraphFullscreen
			? Math.max(width, SUBGRAPH_WIDTH + 32)
			: width;

	if (!currentNodeType) {
		return null;
	}

	return (
		<Draggable
			className="absolute left-0 top-0 cursor-default select-none"
			style={{
				width: nodeWidth,
				transform: `translate(${x}px, ${y}px)`,
			}}
			onDragStart={handleDragStartWithGraph}
			onDrag={handleDrag}
			onDragEnd={stopDrag}
			onMouseDown={() => {
				setSelectedNode(editorId, id);
			}}
			innerRef={nodeWrapper}
			data-node-id={id}
			data-flume-component="node"
			data-flume-node-type={currentNodeType.type}
			data-flume-component-is-root={!!root}
			onContextMenu={handleContextMenu}
			suppressDragPrep={suppressEmbeddedPortControlPrep}
			stageState={stageState}
			stageRect={stageRect}
		>
			<Frame
				className={cn(
					"min-w-0 w-full transition-all duration-150",
					hasError &&
						"ring-2 ring-red-500 shadow-[0_0_12px_rgba(239,68,68,0.4)]",
					isSelected &&
						!hasError &&
						"ring-2 ring-(--acc) shadow-[0_0_16px_color-mix(in_srgb,var(--acc)_35%,transparent)]",
				)}
				data-selected={isSelected ? "true" : undefined}
			>
				<FrameHeader>
					<div className="flex items-center justify-between gap-2">
						<div className="min-w-0 flex-1 flex flex-col">
							{renderNodeHeader ? (
								renderNodeHeader(FrameTitle, currentNodeType, {
									openMenu: handleContextMenu,
									closeMenu: closeContextMenu,
									deleteNode,
								})
							) : (
								<>
									<FrameTitle data-flume-component="node-header">
										{label}
									</FrameTitle>
									{description ? (
										<FrameDescription data-flume-component="node-description">
											{description}
										</FrameDescription>
									) : null}
								</>
							)}
						</div>
						<div className="flex items-center gap-1.5 shrink-0">
							<Badge
								variant={statusMeta.variant}
								size="xxs"
								dot
								pulse={statusMeta.pulse}
								label={statusMeta.label}
								data-flume-node-status={id}
							/>
							<button
								type="button"
								onClick={(event) => {
									event.stopPropagation();
									setLogsOpen((open) => !open);
								}}
								className={cn(
									"flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono tracking-tight transition-colors select-none cursor-pointer",
									logsOpen
										? "bg-(--acc)/20 text-(--acc) border border-(--acc)/40"
										: "text-(--f3) hover:text-(--f1) hover:bg-(--raised)/60 border border-transparent",
								)}
								title={logsOpen ? "Hide logs" : "Show logs"}
								data-flume-logs-toggle={id}
							>
								<TerminalIcon className="size-3" />
								<span>{nodeLogs.length > 0 ? nodeLogs.length : "Logs"}</span>
							</button>
						</div>
					</div>
				</FrameHeader>

				{hasError && (
					<div
						className="bg-red-950/80 border-y border-red-500/40 px-3 py-1.5 text-xs text-red-300 font-mono"
						data-flume-node-error={id}
					>
						{nodeDiagnostics[0].message}
					</div>
				)}

				{nodeResult && (
					<div
						className="bg-emerald-950/80 border-y border-emerald-500/40 px-3 py-1 text-xs text-emerald-300 font-mono"
						data-flume-node-result={id}
					>
						{Object.entries(nodeResult)
							.map(([portName, portValue]) => `${portName}: ${portValue}`)
							.join(" | ")}
					</div>
				)}

				{/*
					A sub-graph's ports are its whole signal's worth of metrics.
					Drawn while it is closed they make the node hundreds of rows
					tall, and there are too many of them across the graph for the
					router to place. Closed, it is a box its edges meet at the
					side; opened, it shows what it is made of.
				*/}
				{showPorts && (
					<Card>
						<CardPanel>
							<Form>
								<Flex.Column gap={4}>
									<IoPorts
										nodeId={id}
										inputs={inputs}
										outputs={outputs}
										connections={connections}
										updateNodeConnections={updateNodeConnections}
										inputData={inputData}
									/>
								</Flex.Column>
							</Form>
						</CardPanel>
					</Card>
				)}

				{isUiComponent && compiledUiNode && (
					<div
						className="border-t border-(--line)/48 p-2.5 bg-(--bg)/60 rounded-b flex flex-col items-center justify-center min-h-[44px]"
						data-flume-ui-preview={id}
					>
						<UIPreviewBoundary
							fallback={
								<span className="text-[10px] font-mono text-(--f3)">
									Preview unavailable
								</span>
							}
						>
							{renderNode(compiledUiNode, id, results)}
						</UIPreviewBoundary>
					</div>
				)}

				{!showPorts && (
					<button
						type="button"
						onClick={() => setSubGraphOpen(true)}
						className="w-full border-t border-(--line)/48 px-3 py-2 text-left text-xs text-(--f3) hover:bg-(--raised)/60 hover:text-(--f1)"
						data-flume-subgraph-summary={id}
					>
						{wiredPortCount} wired{" "}
						{wiredPortCount === 1 ? "connection" : "connections"}
					</button>
				)}

				<NodeLogs
					nodeId={id}
					label={label}
					logs={nodeLogs}
					status={nodeStatus}
					isOpen={logsOpen}
					onClose={() => setLogsOpen(false)}
				/>

				{isBlock && (
					<div className="border-t border-(--line)/48 px-3 py-2">
						<Flex.Row align="center" gap={2}>
							<button
								type="button"
								onClick={() => setSubGraphOpen((open) => !open)}
								className="flex flex-1 items-center gap-1.5 rounded px-2 py-1 text-xs text-(--f3) hover:bg-(--raised)/60 hover:text-(--f1)"
							>
								<NetworkIcon className="size-3.5" />
								{subGraphOpen ? "Collapse operations" : "Expand operations"}
							</button>
							{subGraphOpen && (
								<button
									type="button"
									onClick={() => setSubGraphFullscreen(true)}
									className="flex items-center gap-1 rounded px-2 py-1 text-xs text-(--f3) hover:bg-(--raised)/60 hover:text-(--f1)"
								>
									<Maximize2Icon className="size-3.5" />
									Full screen
								</button>
							)}
						</Flex.Row>
						{subGraphOpen && !subGraphFullscreen && (
							<div
								className="mt-2"
								data-subgraph-editor
								style={{ pointerEvents: "all" }}
							>
								{subGraphEditor}
							</div>
						)}
					</div>
				)}
			</Frame>

			{fullscreenOverlay}

			{portalContainer && menuOpen
				? createPortal(
						<ContextMenu
							x={menuCoordinates.x}
							y={menuCoordinates.y}
							options={[
								...(deletable !== false
									? [
											{
												label: "Delete Node",
												value: "deleteNode",
												description:
													"Deletes a node and all of its connections.",
											},
										]
									: []),
							]}
							onRequestClose={closeContextMenu}
							onOptionSelected={handleMenuOption}
							hideFilter
							label="Node Options"
							emptyText="This node has no options."
						/>,
						portalContainer,
					)
				: null}
		</Draggable>
	);
};

export default Node;
