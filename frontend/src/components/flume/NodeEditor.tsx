import React, { useId } from "react";
import Cache from "#/components/flume/Cache";
import Comment from "#/components/flume/Comment/Comment";
import Connections from "#/components/flume/Connections/Connections";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import {
	DRAG_CONNECTION_ID,
	PORT_LAYER_ID,
	STAGE_ID,
} from "#/components/flume/constants";
import {
	setDragOverride as setDragOverrideInStore,
	useDragOverride,
} from "#/components/flume/flume-editor.store";
import Node from "#/components/flume/Node/Node";
import Stage from "#/components/flume/Stage/Stage";
import { portLayoutKey } from "#/components/flume/spatial-index";
import { useFlumeGraphWorker } from "#/components/flume/useFlumeGraphWorker";
import {
	type PortLayoutRegistrationContext,
	useSpatialIndex,
} from "#/components/flume/useSpatialIndex";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import usePrevious from "#/hooks/usePrevious";
import { cn } from "@/lib/utils";
import { FlumeProviders } from "./FlumeProviders";
import { dispatchGraphLayout, type GraphLayoutMode } from "./graphLayout";
import styles from "./styles.module.css";
import { dispatchFlumeToastAction, type ToastAction } from "./toastsReducer";
import type {
	CircularBehavior,
	DefaultConnection,
	DefaultNode,
	FlumeCommentMap,
	NodeHeaderRenderCallback,
	NodeMap,
	NodeTypeMap,
	PortTypeMap,
} from "./types";
import { useCommentsState, useViewportState } from "./useGraphRowState";
import { useNodesState } from "./useNodesState";

const defaultContext = {};

export type NodeEditorHandle = {
	getNodes: () => NodeMap;
	getComments: () => FlumeCommentMap;
	/**
	 * Insert a starter graph if the row is empty. Idempotent — no-op
	 * if the row already has nodes.
	 */
	seed: (params: {
		defaultNodes?: DefaultNode[];
		defaultConnections?: DefaultConnection[];
	}) => void;
	hasNodes: () => boolean;
};

interface NodeEditorProps {
	ref?: React.Ref<NodeEditorHandle>;
	nodeTypes: NodeTypeMap;
	portTypes: PortTypeMap;
	context?: unknown;
	/**
	 * Persistence is mandatory. Every editor instance (including embedded
	 * sub-editors) reads from and writes to pipelineGraphCollection under
	 * its own id. There is no inline-state mode.
	 */
	graphId: string;
	projectId?: string | null;
	spaceToPan?: boolean;
	hideComments?: boolean;
	disableComments?: boolean;
	disableZoom?: boolean;
	disablePan?: boolean;
	disableFocusCapture?: boolean;
	circularBehavior?: CircularBehavior;
	renderNodeHeader?: NodeHeaderRenderCallback;
	debug?: boolean;
	className?: string;
	edgeRoutingMode?: EdgeRoutingMode;
	graphLayoutMode?: GraphLayoutMode;
	style?: React.CSSProperties;
}

export const NodeEditor = ({
	ref,
	nodeTypes = {},
	portTypes = {},
	context = defaultContext,
	graphId,
	projectId,
	spaceToPan = false,
	hideComments = false,
	disableComments = false,
	disableZoom = false,
	disablePan = false,
	disableFocusCapture = false,
	circularBehavior,
	renderNodeHeader,
	debug,
	className,
	style,
	edgeRoutingMode = "smooth",
	graphLayoutMode = "freeform",
}: NodeEditorProps) => {
	const editorId = useId() ?? "";
	const cache = React.useRef(new Cache());
	const stage = React.useRef<DOMRect | undefined>(undefined);
	const scaleRef = React.useRef(1);
	const environmentRef = React.useRef({
		nodeTypes,
		portTypes,
		cache,
		circularBehavior,
		context,
	});

	environmentRef.current = {
		nodeTypes,
		portTypes,
		cache,
		circularBehavior,
		context,
	};

	const [sideEffectToasts, setSideEffectToasts] = React.useState<ToastAction>();

	const getEnvironment = React.useCallback(() => environmentRef.current, []);

	const {
		nodes,
		actions: nodeActions,
		hasRow,
		seed: seedNodes,
	} = useNodesState({
		graphId,
		projectId,
		nodeTypes,
		portTypes,
		context,
		getEnvironment,
	});

	const { comments, dispatch: dispatchComments } = useCommentsState(graphId);

	const nodeTypeRegistryKey = React.useMemo(
		() => Object.keys(nodeTypes).sort().join("\0"),
		[nodeTypes],
	);
	const portTypeRegistryKey = React.useMemo(
		() => Object.keys(portTypes).sort().join("\0"),
		[portTypes],
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: reconcile graph when node/port registries change
	React.useEffect(() => {
		if (!hasRow) {
			return;
		}

		nodeActions.reconcileNodeTypes();
	}, [hasRow, nodeTypeRegistryKey, portTypeRegistryKey]);

	const visibleNodes = React.useMemo(
		() => Object.values(nodes).filter((node) => nodeTypes[node.type]),
		[nodes, nodeTypes],
	);

	const { viewport: stageState, dispatch: dispatchStageState } =
		useViewportState(graphId);

	React.useLayoutEffect(() => {
		scaleRef.current = stageState.scale;
	}, [stageState.scale]);

	// Ephemeral drag state lives in the Flume store so other panels
	// (e.g. inspector overlays) can subscribe to it too without prop
	// drilling. Keyed per editorId so multiple editors don't collide.
	const dragOverride = useDragOverride(editorId);

	const nodesRef = React.useRef(nodes);
	nodesRef.current = nodes;

	const graphWorkerRef = React.useRef<ReturnType<
		typeof useFlumeGraphWorker
	> | null>(null);

	const onNodeLayoutChange = React.useCallback(
		(nodeId: string, width: number, height: number) => {
			graphWorkerRef.current?.setNodeLayout(nodeId, width, height);
		},
		[],
	);

	const { indexRef, registerPortLayout: registerPortLayoutBase } =
		useSpatialIndex(editorId, nodeActions, onNodeLayoutChange);
	const graphWorkerBase = useFlumeGraphWorker(
		editorId,
		edgeRoutingMode,
		indexRef,
	);
	graphWorkerRef.current = graphWorkerBase;

	const graphWorker = React.useMemo(
		() => ({
			...graphWorkerBase,
			beginDrag: (nodeId: string) => {
				graphWorkerBase.beginDrag(nodeId);
			},
			updateDrag: (nodeId: string, x: number, y: number) => {
				setDragOverrideInStore(editorId, { [nodeId]: { x, y } });
				graphWorkerBase.updateDrag(nodeId, x, y);
			},
			endDrag: (nodeId: string, x: number, y: number) => {
				setDragOverrideInStore(editorId, null);
				graphWorkerBase.endDrag(nodeId, x, y);
			},
		}),
		[graphWorkerBase, editorId],
	);

	// Push topology to the worker whenever nodes change. This is the
	// only place setGraph fires; everything downstream (port layouts,
	// node sizes, routing mode, drag) is handled by incremental setters.
	React.useEffect(() => {
		graphWorkerBase.setGraph(nodes);
	}, [graphWorkerBase, nodes]);

	// Single signal across the editor: "something changed, ask the worker
	// to re-render." All routing math lives in the worker; consumers
	// downstream (IoPorts, Draggable, layout effects) call this when they
	// emit a state mutation. Position-override piggybacking is gone —
	// drag flows through beginDrag/updateDrag/endDrag instead.
	const recalculateConnections = graphWorkerBase.scheduleRender;
	const recalculateConnectionsRef = React.useRef(recalculateConnections);
	recalculateConnectionsRef.current = recalculateConnections;

	const registerPortLayout = React.useCallback<
		NonNullable<React.ContextType<typeof PortLayoutRegistrationContext>>
	>(
		(nodeId, portName, transputType, entry) => {
			const layoutKey = portLayoutKey(nodeId, portName, transputType);
			const previous = indexRef.current.portLayouts.get(layoutKey);

			if (
				previous &&
				previous.offsetX === entry.offsetX &&
				previous.offsetY === entry.offsetY
			) {
				return;
			}

			registerPortLayoutBase(nodeId, portName, transputType, entry);
			graphWorkerBase.setPortLayout(
				nodeId,
				portName,
				transputType,
				entry.offsetX,
				entry.offsetY,
			);
		},
		[indexRef, registerPortLayoutBase, graphWorkerBase],
	);

	const recalculateStageRect = React.useCallback(() => {
		stage.current = document
			.getElementById(`${STAGE_ID}${editorId}`)
			?.getBoundingClientRect();
	}, [editorId]);

	React.useLayoutEffect(() => {
		recalculateConnections();
	}, [recalculateConnections]);

	const triggerRecalculation = recalculateConnections;

	const nodesRefForLayout = nodesRef;

	const prevGraphLayout = usePrevious(graphLayoutMode);

	React.useEffect(() => {
		const mode = graphLayoutMode ?? "freeform";
		if (prevGraphLayout === undefined || prevGraphLayout === mode) return;
		if (mode === "freeform") return;
		dispatchGraphLayout(mode, nodesRefForLayout.current, nodeActions);
		triggerRecalculation();
	}, [graphLayoutMode, prevGraphLayout, triggerRecalculation, nodeActions]);

	React.useImperativeHandle(ref, () => ({
		getNodes: () => {
			return nodes;
		},
		getComments: () => {
			return comments;
		},
		seed: seedNodes,
		hasNodes: () => Object.keys(nodes).length > 0,
	}));

	// Persistence is owned by the collection — no onChange/onCommentsChange
	// fan-out. The collection.update inside useNodesState writes immediately
	// and useLiveQuery subscribers re-render off the same source.

	React.useEffect(() => {
		if (sideEffectToasts) {
			dispatchFlumeToastAction(sideEffectToasts);
			setSideEffectToasts(undefined);
		}
	}, [sideEffectToasts]);

	return (
		<Flex.Column
			className={cn("min-h-0 flex-1", className)}
			style={style}
			fullHeight
			fullWidth
		>
			<FlumeProviders
				value={{
					indexRef,
					registerPortLayout,
					graphWorker,
					dragOverride,
					nodes,
					edgeRoutingMode,
					portTypes,
					nodeTypes,
					nodeActions,
					triggerRecalculation,
					context,
					stageState,
					cache,
					graphId,
					editorId,
					recalculateStageRect,
				}}
			>
				<Stage
					editorId={editorId}
					scale={stageState.scale}
					translate={stageState.translate}
					spaceToPan={spaceToPan}
					disablePan={disablePan}
					disableZoom={disableZoom}
					dispatchStageState={dispatchStageState}
					dispatchComments={dispatchComments}
					disableComments={disableComments || hideComments}
					disableFocusCapture={disableFocusCapture}
					stageRef={stage}
					numNodes={Object.keys(nodes).length}
					outerStageChildren={
						debug ? (
							<div className={styles.debugWrapper}>
								<Button
									type="button"
									variant="outline"
									size="s"
									onClick={() => console.log(nodes)}
								>
									Log Nodes
								</Button>
								<Button
									type="button"
									variant="outline"
									size="s"
									onClick={() => console.log(JSON.stringify(nodes))}
								>
									Export Nodes
								</Button>
								<Button
									type="button"
									variant="outline"
									size="s"
									onClick={() => console.log(comments)}
								>
									Log Comments
								</Button>
							</div>
						) : null
					}
				>
					<div
						className={styles.portLayer}
						id={`${PORT_LAYER_ID}${editorId}`}
					/>
					{!hideComments &&
						Object.values(comments).map((comment) => (
							<Comment
								{...comment}
								stageRect={stage}
								dispatch={dispatchComments}
								onDragStart={recalculateStageRect}
								key={comment.id}
							/>
						))}
					{visibleNodes.map((node) => (
						<Node
							{...node}
							stageRect={stage}
							onDragStart={recalculateStageRect}
							renderNodeHeader={renderNodeHeader}
							key={node.id}
						/>
					))}
					<Connections editorId={editorId} />
					<div
						className={styles.dragWrapper}
						id={`${DRAG_CONNECTION_ID}${editorId}`}
					/>
				</Stage>
			</FlumeProviders>
		</Flex.Column>
	);
};
