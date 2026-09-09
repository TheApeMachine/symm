import { curveBasis, line } from "d3-shape";
import {
	buildRoutingGridFromObstacles,
	routeOrthogonalWithGrid,
} from "#/components/flume/orthogonal-grid-router";
import type { Coordinate } from "#/components/flume/types";

export type EdgeRoutingMode = "smooth" | "straight" | "orthogonal";

/** Axis-aligned bounds in the same stage coordinate space as edge endpoints. */
export type ObstacleRect = {
	readonly left: number;
	readonly right: number;
	readonly top: number;
	readonly bottom: number;
};

const OBSTACLE_PADDING = 16;
const CORRIDOR_MARGIN = 36;
const CORRIDOR_SCAN_STEP = 20;
const CORRIDOR_SCAN_LIMIT = 200;
/** Exit stub length: distance the wire travels horizontally before turning. */
const PORT_EXIT_STUB = 40;

const padObstacle = (o: ObstacleRect, pad: number): ObstacleRect => ({
	left: o.left - pad,
	right: o.right + pad,
	top: o.top - pad,
	bottom: o.bottom + pad,
});

const segmentHitsHorizontal = (
	y: number,
	x1: number,
	x2: number,
	obstacles: ReadonlyArray<ObstacleRect>,
): boolean => {
	if (x1 === x2) return false;
	const [xa, xb] = x1 <= x2 ? [x1, x2] : [x2, x1];

	for (const raw of obstacles) {
		const o = padObstacle(raw, OBSTACLE_PADDING);
		if (y <= o.top || y >= o.bottom) continue;
		if (xb <= o.left || xa >= o.right) continue;
		return true;
	}

	return false;
};

const segmentHitsVertical = (
	x: number,
	y1: number,
	y2: number,
	obstacles: ReadonlyArray<ObstacleRect>,
): boolean => {
	if (y1 === y2) return false;
	const [ya, yb] = y1 <= y2 ? [y1, y2] : [y2, y1];

	for (const raw of obstacles) {
		const o = padObstacle(raw, OBSTACLE_PADDING);
		if (x <= o.left || x >= o.right) continue;
		if (yb <= o.top || ya >= o.bottom) continue;
		return true;
	}

	return false;
};

/*
Orthogonal path from output port to input port.

Port conventions (fixed by UI layout):
  - Output ports are on the RIGHT face of a node → wire exits rightward
  - Input ports are on the LEFT face of a node  → wire enters leftward

Routing order:
  1. Corridor scan — minimal bends, vertical bus at stub midpoint
  2. Grid A* — obstacle avoiding fallback with bend-penalized search
  3. Forced corridor — last resort when grid exhausts its budget
*/
export const calculateOrthogonalEdgePath = (
	from: Coordinate,
	to: Coordinate,
	obstaclesVertical: ReadonlyArray<ObstacleRect>,
	obstaclesHorizontal: ReadonlyArray<ObstacleRect>,
): string => {
	const corridorPath = tryOrthogonalCorridorRoute(
		from,
		to,
		obstaclesVertical,
		obstaclesHorizontal,
	);

	if (corridorPath) {
		return corridorPath;
	}

	const grid = buildRoutingGridFromObstacles(obstaclesHorizontal);
	const gridPath = routeOrthogonalWithGrid(from, to, grid);

	if (gridPath) {
		return gridPath;
	}

	return calculateOrthogonalEdgePathForced(
		from,
		to,
		obstaclesVertical,
		obstaclesHorizontal,
	);
};

const tryOrthogonalCorridorRoute = (
	from: Coordinate,
	to: Coordinate,
	obstaclesVertical: ReadonlyArray<ObstacleRect>,
	obstaclesHorizontal: ReadonlyArray<ObstacleRect>,
): string | null => {
	const px = from.x + PORT_EXIT_STUB;
	const py = from.y;
	const qx = to.x - PORT_EXIT_STUB;
	const qy = to.y;

	if (px < qx) {
		const segment = (busX: number) =>
			`M ${from.x} ${from.y} L ${px} ${py} L ${busX} ${py} L ${busX} ${qy} L ${qx} ${qy} L ${to.x} ${to.y}`;

		const isClear = (busX: number) =>
			!segmentHitsHorizontal(py, px, busX, obstaclesHorizontal) &&
			!segmentHitsHorizontal(qy, busX, qx, obstaclesHorizontal) &&
			!segmentHitsVertical(busX, py, qy, obstaclesVertical);

		const mid = Math.round((px + qx) / 2);

		for (let step = 0; step <= CORRIDOR_SCAN_LIMIT; step++) {
			if (isClear(mid + step * CORRIDOR_SCAN_STEP)) {
				return segment(mid + step * CORRIDOR_SCAN_STEP);
			}

			if (step > 0 && isClear(mid - step * CORRIDOR_SCAN_STEP)) {
				return segment(mid - step * CORRIDOR_SCAN_STEP);
			}
		}

		const midY = Math.round((py + qy) / 2);
		const detourSegment = (routeY: number) =>
			`M ${from.x} ${from.y} L ${px} ${py} L ${px} ${routeY} L ${qx} ${routeY} L ${qx} ${qy} L ${to.x} ${to.y}`;
		const isDetourClear = (routeY: number) =>
			!segmentHitsVertical(px, py, routeY, obstaclesVertical) &&
			!segmentHitsHorizontal(routeY, px, qx, obstaclesHorizontal) &&
			!segmentHitsVertical(qx, routeY, qy, obstaclesVertical);

		for (let step = 0; step <= CORRIDOR_SCAN_LIMIT; step++) {
			if (isDetourClear(midY + step * CORRIDOR_SCAN_STEP)) {
				return detourSegment(midY + step * CORRIDOR_SCAN_STEP);
			}

			if (step > 0 && isDetourClear(midY - step * CORRIDOR_SCAN_STEP)) {
				return detourSegment(midY - step * CORRIDOR_SCAN_STEP);
			}
		}

		return null;
	}

	const eastBus = Math.max(from.x, to.x) + CORRIDOR_MARGIN;
	const westBus = Math.min(from.x, to.x) - CORRIDOR_MARGIN;

	const segment = (busY: number, busXEast: number, busXWest: number) =>
		`M ${from.x} ${from.y} L ${px} ${py} L ${busXEast} ${py} L ${busXEast} ${busY} L ${busXWest} ${busY} L ${busXWest} ${qy} L ${qx} ${qy} L ${to.x} ${to.y}`;

	const allNodes = [...obstaclesVertical, ...obstaclesHorizontal];
	const nodeExtents = allNodes.flatMap((obstacle) => [
		obstacle.top - OBSTACLE_PADDING,
		obstacle.bottom + OBSTACLE_PADDING,
	]);
	const yMin = Math.min(py, qy, ...nodeExtents) - CORRIDOR_MARGIN;
	const yMax = Math.max(py, qy, ...nodeExtents) + CORRIDOR_MARGIN;
	const midY = Math.round((py + qy) / 2);
	const candidates = Array.from(
		new Set<number>([
			midY,
			yMin,
			yMax,
			...nodeExtents.flatMap((extent) => [
				extent - CORRIDOR_MARGIN,
				extent + CORRIDOR_MARGIN,
			]),
		]),
	).sort((left, right) => Math.abs(left - midY) - Math.abs(right - midY));

	const isBypassClear = (busY: number, busXEast: number, busXWest: number) =>
		!segmentHitsHorizontal(py, px, busXEast, obstaclesHorizontal) &&
		!segmentHitsHorizontal(qy, busXWest, qx, obstaclesHorizontal) &&
		!segmentHitsVertical(busXEast, py, busY, obstaclesVertical) &&
		!segmentHitsVertical(busXWest, busY, qy, obstaclesVertical) &&
		!segmentHitsHorizontal(busY, busXWest, busXEast, obstaclesVertical);

	for (let busStep = 0; busStep < CORRIDOR_SCAN_LIMIT; busStep++) {
		const busXEast = eastBus + busStep * CORRIDOR_SCAN_STEP;
		const busXWest = westBus - busStep * CORRIDOR_SCAN_STEP;

		for (const busY of candidates) {
			if (isBypassClear(busY, busXEast, busXWest)) {
				return segment(busY, busXEast, busXWest);
			}
		}
	}

	return null;
};

const calculateOrthogonalEdgePathForced = (
	from: Coordinate,
	to: Coordinate,
	obstaclesVertical: ReadonlyArray<ObstacleRect>,
	_obstaclesHorizontal: ReadonlyArray<ObstacleRect>,
): string => {
	const px = from.x + PORT_EXIT_STUB;
	const py = from.y;
	const qx = to.x - PORT_EXIT_STUB;
	const qy = to.y;

	if (px < qx) {
		const segment = (busX: number) =>
			`M ${from.x} ${from.y} L ${px} ${py} L ${busX} ${py} L ${busX} ${qy} L ${qx} ${qy} L ${to.x} ${to.y}`;

		return segment(Math.round((px + qx) / 2));
	}

	const eastBus = Math.max(from.x, to.x) + CORRIDOR_MARGIN;
	const westBus = Math.min(from.x, to.x) - CORRIDOR_MARGIN;
	const yMin =
		Math.min(
			py,
			qy,
			...obstaclesVertical.flatMap((obstacle) => [
				obstacle.top - OBSTACLE_PADDING,
				obstacle.bottom + OBSTACLE_PADDING,
			]),
		) - CORRIDOR_MARGIN;

	return `M ${from.x} ${from.y} L ${px} ${py} L ${eastBus} ${py} L ${eastBus} ${yMin} L ${westBus} ${yMin} L ${westBus} ${qy} L ${qx} ${qy} L ${to.x} ${to.y}`;
};

const calculateSmoothCurve = (from: Coordinate, to: Coordinate) => {
	const length = to.x - from.x;
	const thirdLength = length / 3;

	let curveCoords: [number, number][] = [];

	if (to.x > from.x - 6) {
		curveCoords = [
			[from.x, from.y],
			[from.x + thirdLength, from.y],
			[from.x + thirdLength * 2, to.y],
			[to.x, to.y],
		];
	} else {
		const outD = 50;
		const height = Math.abs(to.y - from.y);
		const heightThird = height / 3;

		if (to.y > from.y) {
			curveCoords = [
				[from.x, from.y],
				[from.x + outD, from.y],
				[from.x + outD, from.y + heightThird],
				[to.x - outD, to.y - heightThird],
				[to.x - outD, to.y],
				[to.x, to.y],
			];
		} else {
			curveCoords = [
				[from.x, from.y],
				[from.x + outD, from.y],
				[from.x + outD, from.y - heightThird],
				[to.x - outD, to.y + heightThird],
				[to.x - outD, to.y],
				[to.x, to.y],
			];
		}
	}

	const curve = line().curve(curveBasis)(curveCoords);
	return curve ?? "";
};

export const calculateEdgePath = (
	mode: EdgeRoutingMode,
	from: Coordinate,
	to: Coordinate,
	obstaclesVertical?: ReadonlyArray<ObstacleRect>,
	obstaclesHorizontal?: ReadonlyArray<ObstacleRect>,
): string => {
	switch (mode) {
		case "straight":
			return `M ${from.x} ${from.y} L ${to.x} ${to.y}`;
		case "orthogonal": {
			const v = obstaclesVertical ?? [];
			const h = obstaclesHorizontal ?? v;
			return calculateOrthogonalEdgePath(from, to, v, h);
		}
		default:
			return calculateSmoothCurve(from, to);
	}
};
