export type FluidVisualParams = {
	yMin: number;
	yMax: number;
	opacity: number;
	lightingFactor: number;
	shininess: number;
	highlight: number;
	strokeThickness: number;
	contourInterval: number;
	contourStrokeThickness: number;
	cellHardnessFactor: number;
	cameraDistanceFactor: number;
	cameraLiftFactor: number;
};

export const FLUID_VISUAL_STORAGE_KEY = "symm.fluid-visual-params";

export const defaultFluidVisualParams = (): FluidVisualParams => ({
	yMin: -0.3,
	yMax: 0.3,
	opacity: 0.99,
	lightingFactor: 0.15,
	shininess: 1.0,
	highlight: 1.0,
	strokeThickness: 2.0,
	contourInterval: 2.0,
	contourStrokeThickness: 0.1,
	cellHardnessFactor: 1.0,
	cameraDistanceFactor: 0.9,
	cameraLiftFactor: 0.55,
});

export type FluidVisualParamKey = keyof FluidVisualParams;

export type FluidVisualParamSpec = {
	key: FluidVisualParamKey;
	label: string;
	min: number;
	max: number;
	step: number;
};

export const fluidVisualParamSpecs: FluidVisualParamSpec[] = [
	{ key: "yMin", label: "Height floor", min: -0.8, max: 0, step: 0.01 },
	{ key: "yMax", label: "Height ceiling", min: 0, max: 0.8, step: 0.01 },
	{ key: "opacity", label: "Opacity", min: 0.1, max: 1, step: 0.01 },
	{ key: "lightingFactor", label: "Lighting", min: 0, max: 1, step: 0.01 },
	{ key: "shininess", label: "Shininess", min: 0, max: 2, step: 0.05 },
	{ key: "highlight", label: "Highlight", min: 0, max: 2, step: 0.05 },
	{ key: "strokeThickness", label: "Stroke", min: 0, max: 5, step: 0.1 },
	{
		key: "contourInterval",
		label: "Contour gap",
		min: 0.25,
		max: 6,
		step: 0.1,
	},
	{
		key: "contourStrokeThickness",
		label: "Contour weight",
		min: 0,
		max: 1,
		step: 0.01,
	},
	{
		key: "cellHardnessFactor",
		label: "Cell hardness",
		min: 0,
		max: 2,
		step: 0.05,
	},
	{
		key: "cameraDistanceFactor",
		label: "Camera distance",
		min: 0.4,
		max: 1.6,
		step: 0.05,
	},
	{
		key: "cameraLiftFactor",
		label: "Camera lift",
		min: 0.2,
		max: 1.2,
		step: 0.05,
	},
];

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

export const mergeFluidVisualParams = (partial: unknown): FluidVisualParams => {
	const defaults = defaultFluidVisualParams();

	if (!isRecord(partial)) {
		return defaults;
	}

	const merged = { ...defaults };

	for (const spec of fluidVisualParamSpecs) {
		const raw = partial[spec.key];

		if (typeof raw !== "number" || !Number.isFinite(raw)) {
			continue;
		}

		merged[spec.key] = clamp(raw, spec.min, spec.max);
	}

	if (merged.yMax - merged.yMin < 0.01) {
		merged.yMin = defaults.yMin;
		merged.yMax = defaults.yMax;
	}

	return merged;
};

export const loadFluidVisualParams = (): FluidVisualParams => {
	if (typeof window === "undefined") {
		return defaultFluidVisualParams();
	}

	try {
		const raw = window.localStorage.getItem(FLUID_VISUAL_STORAGE_KEY);

		if (raw === null) {
			return defaultFluidVisualParams();
		}

		return mergeFluidVisualParams(JSON.parse(raw) as unknown);
	} catch {
		return defaultFluidVisualParams();
	}
};

export const saveFluidVisualParams = (params: FluidVisualParams): void => {
	if (typeof window === "undefined") {
		return;
	}

	window.localStorage.setItem(FLUID_VISUAL_STORAGE_KEY, JSON.stringify(params));
};
