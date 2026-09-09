import type { RefObject } from "react";
import type FlumeCache from "#/components/flume/Cache";
import { getCanvasRef } from "#/components/flume/connection-stage-coords";
import type { TransputType } from "#/components/flume/types";

/** Encodes a connection-id segment so the `|` delimiter is unambiguous. */
const encodeSegment = (segment: string) =>
	segment.replace(/[|\\]/g, (character) => `\\${character}`);

/** Stable, unambiguous id for a (output, input) port pair. */
export const connectionId = (
	outputNodeId: string,
	outputPortName: string,
	inputNodeId: string,
	inputPortName: string,
) =>
	`${encodeSegment(outputNodeId)}|${encodeSegment(outputPortName)}|${encodeSegment(inputNodeId)}|${encodeSegment(inputPortName)}`;

const portHandleSelector = (
	nodeId: string,
	portName: string,
	transputType: TransputType,
) =>
	`[data-flume-component="port-handle"][data-node-id="${nodeId}"][data-port-name="${portName}"][data-port-transput-type="${transputType}"]`;

export const findPortHandle = (
	root: ParentNode,
	nodeId: string,
	portName: string,
	transputType: TransputType = "input",
) => root.querySelector(portHandleSelector(nodeId, portName, transputType));

const portHandleFromElement = (element: Element): HTMLElement | null => {
	const portHandle = element.closest(
		'[data-flume-component="port-handle"][data-port-name]',
	);

	if (portHandle instanceof HTMLElement) {
		return portHandle;
	}

	return null;
};

/** Finds the port handle under the pointer when a connection drag ends. */
export const resolvePortDropTarget = (
	event: MouseEvent,
): HTMLElement | null => {
	if (
		typeof document !== "undefined" &&
		typeof document.elementsFromPoint === "function"
	) {
		for (const element of document.elementsFromPoint(
			event.clientX,
			event.clientY,
		)) {
			const portHandle = portHandleFromElement(element);

			if (portHandle) {
				return portHandle;
			}
		}
	}

	if (event.target instanceof Element) {
		const portHandle = portHandleFromElement(event.target);

		if (portHandle) {
			return portHandle;
		}
	}

	if (
		typeof document !== "undefined" &&
		typeof document.elementFromPoint === "function"
	) {
		const elementFromPoint = document.elementFromPoint(
			event.clientX,
			event.clientY,
		);

		if (elementFromPoint) {
			return portHandleFromElement(elementFromPoint);
		}
	}

	return null;
};

export const getPortInEditor = (
	editorId: string,
	nodeId: string,
	portName: string,
	transputType: TransputType = "input",
) => {
	const canvas = getCanvasRef(editorId);
	if (!canvas) return null;
	return findPortHandle(canvas, nodeId, portName, transputType);
};

const getPort = (
	nodeId: string,
	portName: string,
	transputType: TransputType = "input",
	editorId?: string,
) => {
	if (editorId) {
		return getPortInEditor(editorId, nodeId, portName, transputType);
	}

	return findPortHandle(document, nodeId, portName, transputType);
};

export const getPortRect = (
	nodeId: string,
	portName: string,
	transputType?: TransputType,
	cache?: RefObject<FlumeCache>,
	editorId?: string,
) => {
	const calculatedTransputType = transputType ?? "input";

	if (cache?.current) {
		const portCacheName = nodeId + portName + calculatedTransputType;
		const cachedPort = cache.current.ports[portCacheName];

		if (cachedPort?.isConnected) {
			return cachedPort.getBoundingClientRect();
		}

		const port = getPort(nodeId, portName, calculatedTransputType, editorId);

		if (port) {
			cache.current.ports[portCacheName] = port;
		}

		return port?.getBoundingClientRect() ?? null;
	}

	const port = getPort(nodeId, portName, calculatedTransputType, editorId);
	return port?.getBoundingClientRect() ?? null;
};
