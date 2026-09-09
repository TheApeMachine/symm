import React from "react";
import {
	getPortRect,
	getStageBounds,
	readLiveStageScale,
	resolvePortDropTarget,
	screenPointToCanvas,
} from "#/components/flume/connectionCalculator";
import {
	EditorIdContext,
	NodeActionsContext,
	PortTypesContext,
	StageContext,
} from "#/components/flume/context";
import type { NodeActions } from "#/components/flume/nodes-actions";
import type { Coordinate, PortTypeMap } from "#/components/flume/types";

/*
usePortDrag isolates the drag-line state machine that used to live inside
the Port component. The Port component now only owns the visual anchor
and portal, while this hook owns:
  - mouse event registration / cleanup
  - dragStartCoordinates / dragCurrentCoordinates state
  - drop resolution (addConnection / removeConnection action calls)
*/

interface PortDragOptions {
	nodeId: string;
	name: string;
	type: string;
	isInput?: boolean;
	triggerRecalculation: () => void;
	portButtonRef: React.RefObject<HTMLButtonElement | null>;
}

type PendingInputDisconnect = {
	inputNodeId: string;
	inputPortName: string;
	outputNodeId: string;
	outputPortName: string;
	connectionElement: SVGPathElement;
};

export interface PortDragHandle {
	isDragging: boolean;
	dragStartCoordinates: Coordinate;
	dragCurrentCoordinates: Coordinate;
	handleDragStart: (e: React.MouseEvent<HTMLButtonElement>) => void;
	beginDragFromPort: () => void;
}

const dispatchAcceptedConnection = ({
	target,
	type,
	nodeId,
	name,
	inputTypes,
	nodeActions,
	triggerRecalculation,
}: {
	target: HTMLElement;
	type: string;
	nodeId: string;
	name: string;
	inputTypes: PortTypeMap;
	nodeActions: NodeActions | null;
	triggerRecalculation: () => void;
}) => {
	const {
		portName: inputPortName,
		nodeId: inputNodeId,
		portType: inputNodeType,
		portTransputType: inputTransputType,
	} = target.dataset;

	if (
		!inputPortName ||
		!inputNodeId ||
		!inputNodeType ||
		!inputTransputType ||
		inputNodeId === nodeId ||
		inputTransputType === "output"
	) {
		return;
	}

	const willAccept =
		inputTypes?.[inputNodeType]?.acceptTypes?.includes(type) ?? false;

	if (!willAccept) {
		return;
	}

	nodeActions?.addConnection(
		{ nodeId: inputNodeId, portName: inputPortName },
		{ nodeId, portName: name },
	);
	triggerRecalculation();
};

const resolveInputDrop = ({
	target,
	outputNodeId,
	outputPortName,
	type,
	inputTypes,
	nodeActions,
	triggerRecalculation,
}: {
	target: HTMLElement;
	outputNodeId: string;
	outputPortName: string;
	type: string;
	inputTypes: PortTypeMap;
	nodeActions: NodeActions | null;
	triggerRecalculation: () => void;
}) => {
	const {
		portName: connectToPortName,
		nodeId: connectToNodeId,
		portType: connectToPortType,
		portTransputType: connectToTransputType,
	} = target.dataset;

	if (
		!connectToPortName ||
		!connectToNodeId ||
		!connectToPortType ||
		!connectToTransputType ||
		outputNodeId === connectToNodeId ||
		connectToTransputType === "output"
	) {
		return;
	}

	const willAccept =
		inputTypes?.[connectToPortType]?.acceptTypes?.includes(type) ?? false;

	if (!willAccept) {
		return;
	}

	nodeActions?.addConnection(
		{ nodeId: connectToNodeId, portName: connectToPortName },
		{ nodeId: outputNodeId, portName: outputPortName },
	);
	triggerRecalculation();
};

export const usePortDrag = ({
	nodeId,
	name,
	type,
	isInput,
	triggerRecalculation,
	portButtonRef,
}: PortDragOptions): PortDragHandle => {
	const nodeActions = React.useContext(NodeActionsContext);
	const stageState = React.useContext(StageContext) || {
		scale: 1,
		translate: { x: 0, y: 0 },
	};
	const editorId = React.useContext(EditorIdContext);
	const inputTypes = React.useContext(PortTypesContext) ?? {};

	const [isDragging, setIsDragging] = React.useState(false);
	const [dragStartCoordinates, setDragStartCoordinates] =
		React.useState<Coordinate>({ x: 0, y: 0 });
	const [dragCurrentCoordinates, setDragCurrentCoordinates] =
		React.useState<Coordinate>({ x: 0, y: 0 });
	const dragFrameRef = React.useRef<number | null>(null);
	const pendingPointerRef = React.useRef<Coordinate | null>(null);
	const pendingInputDisconnectRef = React.useRef<PendingInputDisconnect | null>(
		null,
	);
	const handleDragRef = React.useRef<(event: MouseEvent) => void>(() => {});
	const handleDragEndRef = React.useRef<(event: MouseEvent) => void>(() => {});

	const DRAG_LISTENER_OPTIONS: AddEventListenerOptions = { capture: true };

	const clientPointToCanvasAt = React.useCallback(
		(clientX: number, clientY: number): Coordinate => {
			const stageRect = getStageBounds(editorId);

			if (!stageRect) {
				return { x: 0, y: 0 };
			}

			const scale = readLiveStageScale(editorId, stageState.scale ?? 1);

			return screenPointToCanvas(
				clientX,
				clientY,
				stageRect,
				scale,
				stageState.translate ?? { x: 0, y: 0 },
			);
		},
		[editorId, stageState.scale, stageState.translate],
	);

	const portRectCenterToCanvas = React.useCallback(
		(rect: DOMRect | null | undefined): Coordinate => {
			if (!rect) {
				return { x: 0, y: 0 };
			}

			return clientPointToCanvasAt(
				rect.left + rect.width / 2,
				rect.top + rect.height / 2,
			);
		},
		[clientPointToCanvasAt],
	);

	const clearDragListeners = React.useCallback(() => {
		if (dragFrameRef.current !== null) {
			cancelAnimationFrame(dragFrameRef.current);
			dragFrameRef.current = null;
		}

		document.removeEventListener(
			"mouseup",
			handleDragEndRef.current,
			DRAG_LISTENER_OPTIONS,
		);
		document.removeEventListener(
			"mousemove",
			handleDragRef.current,
			DRAG_LISTENER_OPTIONS,
		);
	}, []);

	const restoreHiddenConnection = React.useCallback(() => {
		const pending = pendingInputDisconnectRef.current;

		if (!pending?.connectionElement.parentElement) {
			pendingInputDisconnectRef.current = null;
			return;
		}

		pending.connectionElement.parentElement.style.visibility = "";
		pendingInputDisconnectRef.current = null;
	}, []);

	const handleDrag = React.useCallback(
		(event: MouseEvent) => {
			pendingPointerRef.current = clientPointToCanvasAt(
				event.clientX,
				event.clientY,
			);

			if (dragFrameRef.current !== null) {
				return;
			}

			dragFrameRef.current = requestAnimationFrame(() => {
				dragFrameRef.current = null;
				const pointer = pendingPointerRef.current;

				if (!pointer) {
					return;
				}

				setDragCurrentCoordinates(pointer);
			});
		},
		[clientPointToCanvasAt],
	);

	handleDragRef.current = handleDrag;

	const handleDragEnd = React.useCallback(
		(event: MouseEvent) => {
			clearDragListeners();

			const pointer = clientPointToCanvasAt(event.clientX, event.clientY);
			setDragCurrentCoordinates(pointer);

			const dropTarget = resolvePortDropTarget(event);
			const pendingInputDisconnect = pendingInputDisconnectRef.current;

			if (isInput && pendingInputDisconnect) {
				nodeActions?.removeConnection(
					{
						nodeId: pendingInputDisconnect.inputNodeId,
						portName: pendingInputDisconnect.inputPortName,
					},
					{
						nodeId: pendingInputDisconnect.outputNodeId,
						portName: pendingInputDisconnect.outputPortName,
					},
				);

				if (dropTarget) {
					resolveInputDrop({
						target: dropTarget,
						outputNodeId: pendingInputDisconnect.outputNodeId,
						outputPortName: pendingInputDisconnect.outputPortName,
						type,
						inputTypes,
						nodeActions,
						triggerRecalculation,
					});
				} else {
					triggerRecalculation();
				}

				pendingInputDisconnectRef.current = null;
			} else if (dropTarget) {
				dispatchAcceptedConnection({
					target: dropTarget,
					type,
					nodeId,
					name,
					inputTypes,
					nodeActions,
					triggerRecalculation,
				});
			}

			setIsDragging(false);
		},
		[
			clearDragListeners,
			clientPointToCanvasAt,
			isInput,
			inputTypes,
			name,
			nodeActions,
			nodeId,
			triggerRecalculation,
			type,
		],
	);

	handleDragEndRef.current = handleDragEnd;

	const attachDragListeners = React.useCallback(() => {
		document.addEventListener(
			"mouseup",
			handleDragEndRef.current,
			DRAG_LISTENER_OPTIONS,
		);
		document.addEventListener(
			"mousemove",
			handleDragRef.current,
			DRAG_LISTENER_OPTIONS,
		);
	}, []);

	const beginDragFromPort = React.useCallback(() => {
		if (isInput) {
			const connectionElement = document.querySelector<SVGPathElement>(
				`[data-input-node-id="${nodeId}"][data-input-port-name="${name}"]`,
			);

			if (!connectionElement?.parentElement) {
				return;
			}

			const {
				inputNodeId = nodeId,
				inputPortName = name,
				outputNodeId = "",
				outputPortName = "",
			} = connectionElement.dataset;

			if (!outputNodeId || !outputPortName) {
				return;
			}

			const outputRect = getPortRect(outputNodeId, outputPortName, "output");
			const coordinates = portRectCenterToCanvas(outputRect);

			pendingInputDisconnectRef.current = {
				inputNodeId,
				inputPortName,
				outputNodeId,
				outputPortName,
				connectionElement,
			};
			connectionElement.parentElement.style.visibility = "hidden";
			setDragStartCoordinates(coordinates);
			setDragCurrentCoordinates(coordinates);
			setIsDragging(true);
			attachDragListeners();
			return;
		}

		const coordinates = portRectCenterToCanvas(
			portButtonRef.current?.getBoundingClientRect(),
		);
		setDragStartCoordinates(coordinates);
		setDragCurrentCoordinates(coordinates);
		setIsDragging(true);
		attachDragListeners();
	}, [
		attachDragListeners,
		isInput,
		name,
		nodeId,
		portButtonRef,
		portRectCenterToCanvas,
	]);

	const handleDragStart = React.useCallback(
		(event: React.MouseEvent<HTMLButtonElement>) => {
			event.preventDefault();
			event.stopPropagation();
			beginDragFromPort();
		},
		[beginDragFromPort],
	);

	React.useEffect(() => {
		return () => {
			clearDragListeners();
			restoreHiddenConnection();
		};
	}, [clearDragListeners, restoreHiddenConnection]);

	return {
		isDragging,
		dragStartCoordinates,
		dragCurrentCoordinates,
		handleDragStart,
		beginDragFromPort,
	};
};
