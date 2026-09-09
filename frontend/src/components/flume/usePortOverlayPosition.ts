import type { RefObject } from "react";
import React from "react";
import { readLiveStageScale } from "#/components/flume/connectionCalculator";
import {
	NodeDragOverrideContext,
	NodeMapContext,
} from "#/components/flume/context";
import { PortLayoutRegistrationContext } from "#/components/flume/useSpatialIndex";

export type PortOverlayPosition = {
	left: number;
	top: number;
	width: number;
	height: number;
	ready: boolean;
};

const hiddenPosition: PortOverlayPosition = {
	left: 0,
	top: 0,
	width: 12,
	height: 12,
	ready: false,
};

const formatOffset = (value: number) => value.toFixed(2);

type MeasuredPortLayout = {
	offsetX: number;
	offsetY: number;
};

/*
Positions port handles from graph state plus measured offsets. Pan and zoom
only change the stage transform — port canvas coordinates stay fixed.
*/
export const usePortOverlayPosition = (
	anchorRef: RefObject<HTMLElement | null>,
	editorId: string,
	nodeId: string,
	portName: string,
	transputType: "input" | "output",
): PortOverlayPosition => {
	const registerPortLayout = React.useContext(PortLayoutRegistrationContext);
	const nodes = React.useContext(NodeMapContext);
	const dragOverride = React.useContext(NodeDragOverrideContext);
	const [portSize, setPortSize] = React.useState({ width: 12, height: 12 });
	const [measuredLayout, setMeasuredLayout] =
		React.useState<MeasuredPortLayout | null>(null);
	const lastRegistrationRef = React.useRef("");

	const node = nodes[nodeId];
	const nodePosition = dragOverride?.[nodeId] ?? {
		x: node?.x ?? 0,
		y: node?.y ?? 0,
	};

	React.useLayoutEffect(() => {
		const measurePortLayout = () => {
			const anchor = anchorRef.current;
			const nodeElement = anchor?.closest('[data-flume-component="node"]');

			if (!anchor || !(nodeElement instanceof HTMLElement) || !node) {
				setMeasuredLayout(null);
				return;
			}

			const scale = readLiveStageScale(editorId, 1);
			const nodeRect = nodeElement.getBoundingClientRect();
			const anchorRect = anchor.getBoundingClientRect();
			const offsetX =
				(anchorRect.left + anchorRect.width / 2 - nodeRect.left) / scale;
			const offsetY =
				(anchorRect.top + anchorRect.height / 2 - nodeRect.top) / scale;
			const nextWidth = Math.max(anchorRect.width / scale, 12);
			const nextHeight = Math.max(anchorRect.height / scale, 12);
			const nextLayout = { offsetX, offsetY };

			setPortSize({ width: nextWidth, height: nextHeight });
			setMeasuredLayout(nextLayout);

			if (!registerPortLayout) {
				return;
			}

			const registrationKey = [
				nodeId,
				portName,
				transputType,
				formatOffset(offsetX),
				formatOffset(offsetY),
			].join("|");

			if (registrationKey === lastRegistrationRef.current) {
				return;
			}

			lastRegistrationRef.current = registrationKey;
			registerPortLayout(nodeId, portName, transputType, nextLayout);
		};

		measurePortLayout();

		const resizeObserver = new ResizeObserver(measurePortLayout);
		const nodeElement = anchorRef.current?.closest(
			'[data-flume-component="node"]',
		);

		if (anchorRef.current) {
			resizeObserver.observe(anchorRef.current);
		}

		if (nodeElement instanceof HTMLElement) {
			resizeObserver.observe(nodeElement);
		}

		window.addEventListener("resize", measurePortLayout);

		return () => {
			resizeObserver.disconnect();
			window.removeEventListener("resize", measurePortLayout);
		};
	}, [
		anchorRef,
		editorId,
		node,
		nodeId,
		portName,
		registerPortLayout,
		transputType,
	]);

	if (!measuredLayout || !node) {
		return hiddenPosition;
	}

	return {
		left: nodePosition.x + measuredLayout.offsetX - portSize.width / 2,
		top: nodePosition.y + measuredLayout.offsetY - portSize.height / 2,
		width: portSize.width,
		height: portSize.height,
		ready: true,
	};
};
