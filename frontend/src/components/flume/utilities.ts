import type { FlumeNode } from "./types";

/** Unique id for graph state (reducers). Use React `useId` for DOM/ARIA in components. */
export function createFlumeId(): string {
	const c = globalThis.crypto;
	if (c && typeof c.randomUUID === "function") {
		return c.randomUUID();
	}
	return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 11)}`;
}

export const checkForCircularNodes = (
	nodes: { [nodeId: string]: FlumeNode },
	startNodeId: string,
) => {
	let isCircular = false;
	const walk = (nodeId: string) => {
		const outputs = Object.values(nodes[nodeId].connections.outputs);
		for (let i = 0; i < outputs.length; i++) {
			if (isCircular) {
				break;
			}
			const outputConnections = outputs[i];
			for (let k = 0; k < outputConnections.length; k++) {
				const connectedTo = outputConnections[k];
				if (connectedTo.nodeId === startNodeId) {
					isCircular = true;
					break;
				} else {
					walk(connectedTo.nodeId);
				}
			}
		}
	};
	walk(startNodeId);
	return isCircular;
};
