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
import {
	ConnectionRecalculateContext,
	DiagnosticsContext,
	FlumeGraphWorkerContext,
	GraphIdContext,
	NodeActionsContext,
	NodeLogsContext,
	type NodeLogEntry,
	NodeResultsContext,
	NodeStatusesContext,
	type NodeStatus,
	NodeTypesContext,
	PortTypesContext,
	StageContext,
} from "#/components/flume/context";
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
import { cn } from "@/lib/utils";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import { fetchAndImportDefinition } from "../import-graph";
import ContextMenu from "../ContextMenu/ContextMenu";
import Draggable from "../Draggable/Draggable";
import IoPorts from "../IoPorts/IoPorts";
import { NodeLogs } from "./NodeLogs";

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
			if (normKey === normType || normType.includes(normKey) || normKey.includes(normType)) {
				return entries;
			}
			const keyParts = normKey.split(".");
			if (normParts.length >= 2 && keyParts.length >= 2 && normParts[0] === keyParts[0]) {
				if (normParts[1].includes(keyParts[1]) || keyParts[1].includes(normParts[1])) {
					return entries;
				}
			}
		}
		return [];
	}, [allLogs, id, type]);

	const [logsOpen, setLogsOpen] = React.useState(false);

	React.useEffect(() => {
		triggerRecalculation?.();
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
			case "init":
			default:
				return { label: "INIT", variant: "disabled" as const, pulse: false };
		}
	}, [nodeStatus]);

	const isDefinitionNode = Boolean(
		String(currentNodeType?.type ?? "").startsWith("definition:") ||
			currentNodeType?.category === "Definitions",
	);

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

	const handleContextMenu = (e: MouseEvent | React.MouseEvent) => {
		e.preventDefault();
		e.stopPropagation();
		setMenuCoordinates({ x: e.clientX, y: e.clientY });
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
		(e: React.MouseEvent<HTMLDivElement>) => {
			if (!(e.target instanceof Element)) return false;
			if (e.target.closest("button, input, textarea, select, option"))
				return true;
			// Suppress only when the click originates inside the nested sub-graph
			// editor, not the outer stage that the block node itself lives in.
			const subgraphContainer = e.currentTarget.querySelector(
				"[data-subgraph-editor]",
			);
			return Boolean(subgraphContainer?.contains(e.target));
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
						<Flex.Row align="center" gap={3} className="border-b px-4 py-2 text-sm text-(--f3)">
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
			<Frame className={`min-w-0 w-full transition-all ${hasError ? "ring-2 ring-red-500 shadow-[0_0_12px_rgba(239,68,68,0.4)]" : ""}`}>
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
								onClick={(e) => {
									e.stopPropagation();
									setLogsOpen((v) => !v);
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
							.map(([k, v]) => `${k}: ${v}`)
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
								onClick={() => setSubGraphOpen((v) => !v)}
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
