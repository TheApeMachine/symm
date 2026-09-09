import React from "react";
import { createPortal } from "react-dom";
import Connection from "#/components/flume/Connection/Connection";
import {
	DRAG_CONNECTION_ID,
	PORT_LAYER_ID,
} from "#/components/flume/constants";
import { EditorIdContext } from "#/components/flume/context";
import type { Colors } from "#/components/flume/types";
import { Button } from "#/components/ui/button";
import { cn } from "@/lib/utils";
import { usePortOverlayPosition } from "../usePortOverlayPosition";
import styles from "./IoPorts.module.css";
import { usePortDrag } from "./usePortDrag";

/*
Port renders the small connection handle that hangs off an input or
output row. The component itself owns only the visual concerns — the
anchor span, the portal'd button, and the optional drag-line preview
created while a connection is being dragged.

All drag state machinery (mouse tracking, REMOVE_CONNECTION/ADD_CONNECTION
dispatch, edge-path geometry against the routing mode) lives in
usePortDrag so this shell can stay focused on layout.
*/

interface PortProps {
	color: Colors;
	name: string;
	type: string;
	isInput?: boolean;
	nodeId: string;
	triggerRecalculation: () => void;
}

const Port = ({
	color = "grey",
	name = "",
	type,
	isInput,
	nodeId,
	triggerRecalculation,
}: PortProps) => {
	const editorId = React.useContext(EditorIdContext);
	const portAnchor = React.useRef<HTMLSpanElement>(null);
	const portButtonRef = React.useRef<HTMLButtonElement>(null);

	const [portLayerHost, setPortLayerHost] = React.useState<HTMLElement | null>(
		null,
	);
	const [dragPortalHost, setDragPortalHost] =
		React.useState<HTMLElement | null>(null);

	React.useLayoutEffect(() => {
		setPortLayerHost(document.getElementById(`${PORT_LAYER_ID}${editorId}`));
		setDragPortalHost(
			document.getElementById(`${DRAG_CONNECTION_ID}${editorId}`),
		);
	}, [editorId]);

	const overlayPosition = usePortOverlayPosition(
		portAnchor,
		editorId,
		nodeId,
		name,
		isInput ? "input" : "output",
	);

	const {
		isDragging,
		dragStartCoordinates,
		dragCurrentCoordinates,
		handleDragStart,
		beginDragFromPort,
	} = usePortDrag({
		nodeId,
		name,
		type,
		isInput,
		triggerRecalculation,
		portButtonRef,
	});

	return (
		<span className="inline-flex shrink-0">
			<span
				ref={portAnchor}
				aria-hidden
				className="inline-block h-3 min-h-3 min-w-3 shrink-0"
			/>
			{portLayerHost && overlayPosition.ready
				? createPortal(
						<Button
							ref={portButtonRef}
							type="button"
							size="sm"
							variant="ghost"
							className={cn(
								"absolute gap-0 rounded-full border-none p-0 shadow-md ring-offset-background [&]:before:shadow-none!",
								"[&]:hover:bg-transparent!",
								"[&]:data-pressed:bg-transparent!",
								styles.port,
							)}
							style={{
								position: "absolute",
								left: overlayPosition.left,
								top: overlayPosition.top,
								width: overlayPosition.width,
								height: overlayPosition.height,
							}}
							onMouseDown={handleDragStart}
							onKeyDown={(event) => {
								if (event.key === "Enter" || event.key === " ") {
									event.preventDefault();
									beginDragFromPort();
								}
							}}
							aria-label={`Connect port ${name}`}
							data-port-color={color}
							data-port-name={name}
							data-port-type={type}
							data-port-transput-type={isInput ? "input" : "output"}
							data-node-id={nodeId}
							data-flume-component="port-handle"
							onDragStart={(event) => {
								event.preventDefault();
								event.stopPropagation();
							}}
						/>,
						portLayerHost,
					)
				: null}
			{isDragging && dragPortalHost
				? createPortal(
						<Connection
							from={dragStartCoordinates}
							to={dragCurrentCoordinates}
						/>,
						dragPortalHost,
					)
				: null}
		</span>
	);
};

export default Port;
