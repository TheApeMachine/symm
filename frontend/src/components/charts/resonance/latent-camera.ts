import {
	type LatentMarkerSizes,
	type LatentPoint3D,
	latentMarkerSizes,
} from "#/components/charts/resonance/resonance-xray-frame";

export type LatentCameraFrame = {
	target: LatentPoint3D;
	orbit: number;
	markerSizes: LatentMarkerSizes;
	cameraOffset: LatentPoint3D;
};

export type LatentWorldCameraFrame = {
	visibleRange: {
		x: { min: number; max: number };
		y: { min: number; max: number };
		z: { min: number; max: number };
	};
	worldCenter: LatentPoint3D;
	worldDimensions: LatentPoint3D;
	cameraPosition: LatentPoint3D;
	markerSizes: LatentMarkerSizes;
};

const MARKER_MARGIN_FACTOR = 2.75;
const VIEWPORT_INSET_FACTOR = 0.32;
const ORBIT_PADDING_FACTOR = 1.48;

const maxAxisExtentFromTarget = (
	points: LatentPoint3D[],
	target: LatentPoint3D,
): number => {
	let maxAxisExtent = 0;

	for (const point of points) {
		maxAxisExtent = Math.max(
			maxAxisExtent,
			Math.abs(point.x - target.x),
			Math.abs(point.y - target.y),
			Math.abs(point.z - target.z),
		);
	}

	return maxAxisExtent;
};

const cameraFrameFromTarget = (
	target: LatentPoint3D,
	points: LatentPoint3D[],
	minOrbitRadius: number,
): LatentCameraFrame => {
	const halfExtent = Math.max(
		maxAxisExtentFromTarget(points, target),
		minOrbitRadius * 0.5,
	);
	const provisionalOrbit = Math.max(halfExtent * 2, minOrbitRadius);
	const provisionalMarkerSizes = latentMarkerSizes(provisionalOrbit);
	const markerMargin = provisionalMarkerSizes.head * MARKER_MARGIN_FACTOR;
	const viewportInset = halfExtent * VIEWPORT_INSET_FACTOR;
	const orbit =
		Math.max(halfExtent + markerMargin, minOrbitRadius) * ORBIT_PADDING_FACTOR +
		viewportInset;

	return {
		target,
		orbit,
		markerSizes: latentMarkerSizes(orbit),
		cameraOffset: {
			x: -0.68,
			y: 0.48,
			z: 0.68,
		},
	};
};

/*
latentCameraFrame keeps the current latent head centered with enough inset
for marker radius and perspective clipping.
*/
export const latentCameraFrame = (
	points: LatentPoint3D[],
	minOrbitRadius = 0.75,
): LatentCameraFrame | null => {
	const head = points.at(-1);

	if (head === undefined) {
		return null;
	}

	return cameraFrameFromTarget(head, points, minOrbitRadius);
};

/*
constellationCameraFrame frames all symbols around the cloud centroid.
*/
export const constellationCameraFrame = (
	points: LatentPoint3D[],
	minOrbitRadius = 0.75,
): LatentCameraFrame | null => {
	if (points.length === 0) {
		return null;
	}

	let minX = points[0].x;
	let maxX = points[0].x;
	let minY = points[0].y;
	let maxY = points[0].y;
	let minZ = points[0].z;
	let maxZ = points[0].z;

	for (let pointIndex = 1; pointIndex < points.length; pointIndex += 1) {
		const point = points[pointIndex];

		minX = Math.min(minX, point.x);
		maxX = Math.max(maxX, point.x);
		minY = Math.min(minY, point.y);
		maxY = Math.max(maxY, point.y);
		minZ = Math.min(minZ, point.z);
		maxZ = Math.max(maxZ, point.z);
	}

	const target = {
		x: (minX + maxX) / 2,
		y: (minY + maxY) / 2,
		z: (minZ + maxZ) / 2,
	};

	return cameraFrameFromTarget(target, points, minOrbitRadius);
};

/*
latentWorldCameraFrame maps data-space framing into SciChart world coordinates.
Camera position and target must be world-space, not raw latent values.
*/
export const latentWorldCameraFrame = (
	frame: LatentCameraFrame,
): LatentWorldCameraFrame => {
	const { target, orbit, markerSizes, cameraOffset } = frame;
	const worldSpan = orbit * 2;

	return {
		visibleRange: {
			x: { min: target.x - orbit, max: target.x + orbit },
			y: { min: target.y - orbit, max: target.y + orbit },
			z: { min: target.z - orbit, max: target.z + orbit },
		},
		worldCenter: { x: orbit, y: orbit, z: orbit },
		worldDimensions: { x: worldSpan, y: worldSpan, z: worldSpan },
		cameraPosition: {
			x: orbit + cameraOffset.x * orbit,
			y: orbit + cameraOffset.y * orbit,
			z: orbit + cameraOffset.z * orbit,
		},
		markerSizes,
	};
};
