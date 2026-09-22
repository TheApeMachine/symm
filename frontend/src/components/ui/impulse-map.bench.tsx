import { renderToStaticMarkup } from "react-dom/server";
import { bench, describe } from "vitest";
import { ImpulseMap, type ImpulsePoint } from "./impulse-map";

// A 15 by 8 measured-grid-shaped fixture exercises full node and edge rendering.
const points: ImpulsePoint[] = Array.from({ length: 120 }, (_, index) => ({
	id: index,
	source: "fixture",
	label: `point-${index}`,
	x: (index % 15) * 30,
	y: Math.floor(index / 15) * 30,
	snr: index / 120,
	activation: index / 120,
	energy: index,
	authority: 1,
	present: true,
}));
const connections = points
	.slice(1)
	.map((point) => ({ from: point.id - 1, to: point.id }));

describe("ImpulseMap", () => {
	bench("render supplied grid and connections", () => {
		renderToStaticMarkup(
			<ImpulseMap points={points} connections={connections} />,
		);
	});
});
