import { describe, expect, it } from "vitest";
import {
	constellationCameraFrame,
	latentCameraFrame,
	latentWorldCameraFrame,
} from "#/components/charts/resonance/latent-camera";
import { latentMarkerSizes } from "#/components/charts/resonance/resonance-xray-frame";

describe("latentCameraFrame", () => {
	it("centers the camera on the current head point", () => {
		const frame = latentCameraFrame([
			{ x: 0.1, y: 0.2, z: 0.3 },
			{ x: 0.4, y: 0.5, z: 0.6 },
		]);

		expect(frame?.target).toEqual({ x: 0.4, y: 0.5, z: 0.6 });
	});

	it("expands orbit to keep the head marker inside the viewport", () => {
		const frame = latentCameraFrame([{ x: 2.5, y: -1.25, z: 0.75 }]);
		const markerSizes = latentMarkerSizes(frame?.orbit ?? 0);

		expect(frame?.orbit ?? 0).toBeGreaterThan(markerSizes.head * 2 + 0.75);
	});

	it("frames a multi-symbol constellation around the cloud centroid", () => {
		const frame = constellationCameraFrame([
			{ x: 0, y: 0, z: 0 },
			{ x: 2, y: 1, z: -1 },
			{ x: -1, y: 0.5, z: 1.5 },
		]);

		expect(frame?.target.x).toBeCloseTo(0.5, 2);
		expect(frame?.orbit ?? 0).toBeGreaterThan(1);
	});

	it("keeps an outlier symbol inside the visible orbit", () => {
		const frame = constellationCameraFrame([
			{ x: 0, y: 0, z: 0 },
			{ x: 0.1, y: -0.1, z: 0.05 },
			{ x: 40, y: 0, z: 0 },
		]);

		expect(frame?.orbit ?? 0).toBeGreaterThan(20);
	});
});

describe("latentWorldCameraFrame", () => {
	it("maps data-space targets into SciChart world coordinates", () => {
		const frame = latentCameraFrame([{ x: 12, y: -4, z: 8 }]);

		expect(frame).not.toBeNull();

		const worldFrame = latentWorldCameraFrame(frame as NonNullable<typeof frame>);

		expect(worldFrame.worldCenter).toEqual({
			x: frame?.orbit,
			y: frame?.orbit,
			z: frame?.orbit,
		});
		expect(worldFrame.cameraPosition.x).not.toBe(frame?.target.x);
		expect(worldFrame.visibleRange.x.min).toBeCloseTo(
			(frame?.target.x ?? 0) - (frame?.orbit ?? 0),
		);
	});
});

describe("latentMarkerSizes", () => {
	it("keeps the head marker smaller than the visible orbit", () => {
		const markerSizes = latentMarkerSizes(1);

		expect(markerSizes.head).toBeLessThan(0.05);
		expect(markerSizes.trail).toBeLessThan(markerSizes.head);
	});
});
