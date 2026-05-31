import { bench, describe } from "vitest";

import {
	projectFluidGridToHeightmap,
	smoothHeightmapSpatial,
} from "#/lib/symm/fluid-grid";

const sampleGrid = {
	heights: Array.from({ length: 32 }, (_, zIndex) =>
		Array.from({ length: 32 }, (_, xIndex) =>
			zIndex === 16 && xIndex === 16 ? 12 : Math.sin(zIndex * 0.2) * 2 + 2,
		),
	),
	min: 0,
	max: 12,
	filledCells: 32 * 32,
	outliers: {
		clippedCount: 1,
		clippedAt: 10,
		rawMax: 12,
		displayMax: 12,
	},
};

describe("fluid-grid surface projection", () => {
	bench("projectFluidGridToHeightmap 50x50", () => {
		projectFluidGridToHeightmap(sampleGrid, 50, 50, -0.3, 0.3);
	});

	bench("smoothHeightmapSpatial 50x50", () => {
		const heightmap = Array.from({ length: 50 }, () =>
			Array.from({ length: 50 }, () => 0.2),
		);

		smoothHeightmapSpatial(heightmap, 3);
	});
});
