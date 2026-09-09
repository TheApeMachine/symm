import React from "react";
import { getCanvasRef } from "#/components/flume/connectionCalculator";
import type { NodeActions } from "#/components/flume/nodes-actions";
import {
	createSpatialIndexSnapshot,
	type PortLayoutEntry,
	portLayoutKey,
	type SpatialIndexSnapshot,
} from "#/components/flume/spatial-index";
export type { SpatialIndexSnapshot };

const defaultSpatialIndex: React.RefObject<SpatialIndexSnapshot> = {
	current: createSpatialIndexSnapshot(),
};

export const SpatialIndexContext =
	React.createContext<React.RefObject<SpatialIndexSnapshot>>(
		defaultSpatialIndex,
	);

export type RegisterPortLayout = (
	nodeId: string,
	portName: string,
	transputType: "input" | "output",
	entry: PortLayoutEntry,
) => void;

export const PortLayoutRegistrationContext =
	React.createContext<RegisterPortLayout | null>(null);

/*
useSpatialIndex maintains node dimensions and port offsets in canvas space.
ResizeObserver updates happen outside the drag hot path; connection routing
reads only from this in-memory index.
*/
export function useSpatialIndex(
	editorId: string,
	nodeActions: NodeActions | null,
	onNodeLayoutChange?: (nodeId: string, width: number, height: number) => void,
): {
	indexRef: React.RefObject<SpatialIndexSnapshot>;
	registerPortLayout: RegisterPortLayout;
} {
	const indexRef = React.useRef<SpatialIndexSnapshot>(
		createSpatialIndexSnapshot(),
	);

	const registerPortLayout = React.useCallback<RegisterPortLayout>(
		(nodeId, portName, transputType, entry) => {
			indexRef.current.portLayouts.set(
				portLayoutKey(nodeId, portName, transputType),
				entry,
			);
		},
		[],
	);

	React.useEffect(() => {
		const canvas = getCanvasRef(editorId);
		if (!canvas) {
			return;
		}

		const measureNode = (element: Element) => {
			const nodeId = element.getAttribute("data-node-id");
			if (!nodeId) {
				return;
			}

			const width = element.clientWidth;
			const height = element.clientHeight;

			if (width <= 0 || height <= 0) {
				return;
			}

			indexRef.current.nodeLayouts.set(nodeId, { width, height });
			onNodeLayoutChange?.(nodeId, width, height);
			nodeActions?.setNodeDimensions({ nodeId, width, height });
		};

		const resizeObserver = new ResizeObserver((entries) => {
			for (const entry of entries) {
				measureNode(entry.target);
			}
		});

		const attachAll = (root: Element) => {
			for (const element of root.querySelectorAll(
				'[data-flume-component="node"][data-node-id]',
			)) {
				resizeObserver.observe(element);
				measureNode(element);
			}
		};

		const mutationObserver = new MutationObserver((records) => {
			for (const record of records) {
				for (const node of record.addedNodes) {
					if (!(node instanceof Element)) {
						continue;
					}

					if (node.matches('[data-flume-component="node"][data-node-id]')) {
						resizeObserver.observe(node);
						measureNode(node);
						continue;
					}

					attachAll(node);
				}

				for (const node of record.removedNodes) {
					if (!(node instanceof Element)) {
						continue;
					}

					const nodeId = node.getAttribute("data-node-id");

					if (!nodeId) {
						continue;
					}

					resizeObserver.unobserve(node);
					indexRef.current.nodeLayouts.delete(nodeId);

					for (const portKey of indexRef.current.portLayouts.keys()) {
						if (portKey.startsWith(`${nodeId}|`)) {
							indexRef.current.portLayouts.delete(portKey);
						}
					}
				}
			}
		});

		attachAll(canvas);
		mutationObserver.observe(canvas, { childList: true, subtree: true });

		return () => {
			resizeObserver.disconnect();
			mutationObserver.disconnect();
			indexRef.current = createSpatialIndexSnapshot();
		};
	}, [editorId, nodeActions, onNodeLayoutChange]);

	return { indexRef, registerPortLayout };
}

/*
ObstacleIndexContext is retained for compatibility; it now aliases spatial index.
*/
export type ObstacleIndex = SpatialIndexSnapshot["nodeLayouts"];

export const ObstacleIndexContext = SpatialIndexContext;
