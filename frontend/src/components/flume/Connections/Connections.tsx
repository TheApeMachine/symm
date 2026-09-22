import React from "react";
import { CONNECTIONS_ID } from "#/components/flume/constants";
import { useSelectedNode } from "#/components/flume/flume-editor.store";
import styles from "./Connections.module.css";

interface ConnectionsProps {
	editorId: string;
}

const Connections = ({ editorId }: ConnectionsProps) => {
	const selectedNodeId = useSelectedNode(editorId);

	React.useEffect(() => {
		const container = document.getElementById(`${CONNECTIONS_ID}${editorId}`);

		if (!container) {
			return;
		}

		if (selectedNodeId) {
			container.setAttribute("data-has-selection", "true");
		}

		if (!selectedNodeId) {
			container.removeAttribute("data-has-selection");
		}

		const paths = container.querySelectorAll<SVGPathElement>(
			"path[data-connection-id]",
		);

		for (const path of paths) {
			const isIncident =
				selectedNodeId !== null &&
				(path.getAttribute("data-output-node-id") === selectedNodeId ||
					path.getAttribute("data-input-node-id") === selectedNodeId);

			if (isIncident) {
				path.setAttribute("data-highlighted", "true");

				if (path.parentElement) {
					path.parentElement.style.zIndex = "10";
				}
			}

			if (!isIncident) {
				path.removeAttribute("data-highlighted");

				if (path.parentElement) {
					path.parentElement.style.zIndex = "0";
				}
			}
		}
	}, [editorId, selectedNodeId]);

	const sanitizedSelectedId = selectedNodeId
		? selectedNodeId.replace(/["\\]/g, "\\$&")
		: null;

	return (
		<div className={styles.svgWrapper} id={`${CONNECTIONS_ID}${editorId}`}>
			{sanitizedSelectedId ? (
				<style>{`
					#${CONNECTIONS_ID}${editorId} svg:has(> [data-output-node-id="${sanitizedSelectedId}"]),
					#${CONNECTIONS_ID}${editorId} svg:has(> [data-input-node-id="${sanitizedSelectedId}"]) {
						z-index: 10 !important;
					}
					#${CONNECTIONS_ID}${editorId} path[data-output-node-id="${sanitizedSelectedId}"],
					#${CONNECTIONS_ID}${editorId} path[data-input-node-id="${sanitizedSelectedId}"],
					#${CONNECTIONS_ID}${editorId} path[data-highlighted="true"] {
						stroke: var(--acc) !important;
						stroke-width: 4px !important;
						filter: drop-shadow(0 0 5px color-mix(in srgb, var(--acc) 70%, transparent));
						transition: stroke 0.15s ease, stroke-width 0.15s ease;
					}
					#${CONNECTIONS_ID}${editorId}[data-has-selection="true"] path[data-connection-id]:not([data-output-node-id="${sanitizedSelectedId}"]):not([data-input-node-id="${sanitizedSelectedId}"]) {
						opacity: 0.35;
						transition: opacity 0.15s ease;
					}
				`}</style>
			) : null}
		</div>
	);
};

export default Connections;
