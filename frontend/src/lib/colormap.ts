/*
colormap maps a normalized 0-1 intensity to the SYMM terminal heat palette.
The stops mirror the mockup in frontend/tmp: sunken brown through navy and teal
into accent gold so low cross-section strength reads cool and high reads hot.
*/
export const DEFAULT_ACCENT_HEX = "#e8a33d";

const clamp01 = (value: number) => Math.max(0, Math.min(1, value));

/*
lerpRgb linearly blends two RGB triples.
*/
const lerpRgb = (
	left: readonly [number, number, number],
	right: readonly [number, number, number],
	t: number,
): [number, number, number] => [
	left[0] + (right[0] - left[0]) * t,
	left[1] + (right[1] - left[1]) * t,
	left[2] + (right[2] - left[2]) * t,
];

/*
hexToRgb parses a #RRGGBB accent into channel values for colormap stops.
*/
export const hexToRgb = (hex: string): [number, number, number] => [
	Number.parseInt(hex.slice(1, 3), 16),
	Number.parseInt(hex.slice(3, 5), 16),
	Number.parseInt(hex.slice(5, 7), 16),
];

/*
colormap returns the heatmap RGB triple for one normalized strength value.
*/
export const colormap = (
	t: number,
	accentHex = DEFAULT_ACCENT_HEX,
): [number, number, number] => {
	const accent = hexToRgb(accentHex);
	const stops: [number, [number, number, number]][] = [
		[0, [14, 12, 10]],
		[0.4, [26, 34, 50]],
		[0.6, [42, 106, 129]],
		[0.8, accent],
		[1, lerpRgb(accent, [255, 248, 224], 0.6)],
	];
	const clamped = clamp01(t);

	for (let index = 0; index < stops.length - 1; index += 1) {
		const [leftStop, leftColor] = stops[index];
		const [rightStop, rightColor] = stops[index + 1];

		if (clamped <= rightStop) {
			const localT = (clamped - leftStop) / (rightStop - leftStop);

			return lerpRgb(leftColor, rightColor, localT);
		}
	}

	return stops[stops.length - 1][1];
};

/*
colormapCss renders one heatmap cell background as an rgb() string.
*/
export const colormapCss = (
	t: number,
	accentHex = DEFAULT_ACCENT_HEX,
): string => {
	const [red, green, blue] = colormap(t, accentHex);

	return `rgb(${red | 0},${green | 0},${blue | 0})`;
};

/*
heatmapForeground picks label contrast against the colormap background.
*/
export const heatmapForeground = (t: number): string =>
	t > 0.62 ? "#14110f" : "var(--f3)";
