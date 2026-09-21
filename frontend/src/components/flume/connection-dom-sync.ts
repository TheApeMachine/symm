import type {
	EdgeRoutingMode,
	ObstacleRect,
} from "#/components/flume/connection-path-math";
import { calculateEdgePath } from "#/components/flume/connection-path-math";
import { getStageRef } from "#/components/flume/connection-stage-coords";
import type { Coordinate } from "#/components/flume/types";

export const deleteConnection = ({ id }: { id: string }) => {
	const line = document.querySelector(`[data-connection-id="${id}"]`);

	if (!line) {
		return;
	}

	const parent = line.parentElement;

	if (
		parent &&
		parent.tagName.toLowerCase() === "svg" &&
		parent.hasAttribute("data-connection-wrapper")
	) {
		parent.remove();
		return;
	}

	line.remove();
};

export const deleteConnectionsByNodeId = (nodeId: string) => {
	const lines = Array.from(
		document.querySelectorAll(
			`[data-output-node-id="${nodeId}"], [data-input-node-id="${nodeId}"]`,
		),
	);

	for (const line of lines) {
		const parent = line.parentElement;

		if (
			parent &&
			parent.tagName.toLowerCase() === "svg" &&
			parent.hasAttribute("data-connection-wrapper")
		) {
			parent.remove();
			continue;
		}

		line.remove();
	}
};

export const updateConnection = ({
	line,
	from,
	to,
	routingMode = "smooth",
	obstaclesVertical,
	obstaclesHorizontal,
}: {
	line: SVGPathElement;
	from: Coordinate;
	to: Coordinate;
	routingMode?: EdgeRoutingMode;
	obstaclesVertical?: ReadonlyArray<ObstacleRect>;
	obstaclesHorizontal?: ReadonlyArray<ObstacleRect>;
}) => {
	line.setAttribute(
		"d",
		calculateEdgePath(
			routingMode,
			from,
			to,
			obstaclesVertical,
			obstaclesHorizontal,
		),
	);
};

/*
ConnectionShellDescriptor matches the worker's roster entry: just the
endpoint identifiers needed to find or create the SVG path element. The
actual d attribute is set separately by applyPaths from the worker
output.
*/
export type ConnectionShellDescriptor = {
	id: string;
	outputNodeId: string;
	outputPortName: string;
	inputNodeId: string;
	inputPortName: string;
};

const PATH_STROKE = "rgb(185, 186, 189)";
const PATH_STROKE_WIDTH = "3";

const getRootSvg = (stage: Element): SVGElement => {
	if (stage.tagName.toLowerCase() === "svg") {
		return stage as SVGElement;
	}

	let rootSvg = stage.querySelector<SVGSVGElement>("svg[data-connections-root]");

	if (!rootSvg) {
		rootSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
		rootSvg.setAttribute("data-connections-root", "true");
		rootSvg.setAttribute(
			"style",
			"position:absolute;left:0;top:0;width:100%;height:100%;pointer-events:none;z-index:0;overflow:visible;",
		);
		stage.appendChild(rootSvg);
	}

	return rootSvg;
};

const createPathElement = (
	descriptor: ConnectionShellDescriptor,
	routingMode: EdgeRoutingMode,
	initialD = "",
): SVGPathElement => {
	const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
	path.setAttribute("d", initialD);
	path.setAttribute("stroke", PATH_STROKE);
	path.setAttribute("stroke-width", PATH_STROKE_WIDTH);
	path.setAttribute("stroke-linecap", "round");

	if (routingMode === "orthogonal") {
		path.setAttribute("stroke-linejoin", "miter");
	}

	path.setAttribute("fill", "none");
	path.setAttribute("data-connection-id", descriptor.id);
	path.setAttribute("data-output-node-id", descriptor.outputNodeId);
	path.setAttribute("data-output-port-name", descriptor.outputPortName);
	path.setAttribute("data-input-node-id", descriptor.inputNodeId);
	path.setAttribute("data-input-port-name", descriptor.inputPortName);

	return path;
};

/*
syncConnectionElements ensures the SVG path elements in the stage exactly
match the roster from the worker. Adds missing elements with an empty d
attribute (worker fills it in), removes stale ones. No geometry math
runs here — that's the worker's job.
*/
export const syncConnectionElements = (
	roster: ReadonlyArray<ConnectionShellDescriptor>,
	editorId: string,
	routingMode: EdgeRoutingMode = "smooth",
): void => {
	const stage = getStageRef(editorId);

	if (!stage) {
		return;
	}

	const rootSvg = getRootSvg(stage);
	const rosterById = new Map<string, ConnectionShellDescriptor>();

	for (const entry of roster) {
		rosterById.set(entry.id, entry);
	}

	const existingElements = new Map<string, SVGPathElement>();

	for (const pathElement of rootSvg.querySelectorAll<SVGPathElement>(
		"[data-connection-id]",
	)) {
		const id = pathElement.getAttribute("data-connection-id");

		if (id) {
			existingElements.set(id, pathElement);
		}
	}

	for (const [id, pathElement] of existingElements) {
		if (!rosterById.has(id)) {
			const parent = pathElement.parentElement as Element | null;

			if (
				parent &&
				parent !== rootSvg &&
				parent.tagName.toLowerCase() === "svg"
			) {
				parent.remove();
			}

			if (
				!parent ||
				parent === rootSvg ||
				parent.tagName.toLowerCase() !== "svg"
			) {
				pathElement.remove();
			}

			existingElements.delete(id);
		}
	}

	for (const entry of roster) {
		const existing = existingElements.get(entry.id);

		if (existing) {
			if (routingMode === "orthogonal") {
				existing.setAttribute("stroke-linejoin", "miter");
			}

			if (routingMode !== "orthogonal") {
				existing.removeAttribute("stroke-linejoin");
			}
			continue;
		}

		const path = createPathElement(entry, routingMode);
		rootSvg.appendChild(path);
	}
};

/*
createSVG builds and appends a single fully-populated path element. Used
by the legacy main-thread create path (still kept for ad-hoc cases like
the drag-line preview that doesn't go through the worker).
*/
export const createSVG = ({
	from,
	to,
	stage,
	id,
	outputNodeId,
	outputPortName,
	inputNodeId,
	inputPortName,
	routingMode = "smooth",
	obstaclesVertical,
	obstaclesHorizontal,
}: {
	from: Coordinate;
	to: Coordinate;
	stage: HTMLElement | SVGElement;
	id: string;
	outputNodeId: string;
	outputPortName: string;
	inputNodeId: string;
	inputPortName: string;
	routingMode?: EdgeRoutingMode;
	obstaclesVertical?: ReadonlyArray<ObstacleRect>;
	obstaclesHorizontal?: ReadonlyArray<ObstacleRect>;
}) => {
	const curve = calculateEdgePath(
		routingMode,
		from,
		to,
		obstaclesVertical,
		obstaclesHorizontal,
	);
	const rootSvg = getRootSvg(stage);
	const path = createPathElement(
		{ id, outputNodeId, outputPortName, inputNodeId, inputPortName },
		routingMode,
		curve,
	);
	rootSvg.appendChild(path);
	return path;
};
