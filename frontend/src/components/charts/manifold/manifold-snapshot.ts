import type {
	ManifoldCarrierRow,
	ManifoldFieldSnapshot,
	ManifoldReadingRow,
} from "#/components/charts/manifold/types";

const readNumber = (value: unknown): number => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}

	if (typeof value === "string" && value.trim() !== "") {
		const parsed = Number(value);

		if (Number.isFinite(parsed)) {
			return parsed;
		}
	}

	return 0;
};

const parseReading = (raw: unknown): ManifoldReadingRow => {
	const row =
		typeof raw === "object" && raw !== null
			? (raw as Record<string, unknown>)
			: {};

	return {
		pressure_grad_x: readNumber(row.pressure_grad_x),
		pressure_grad_y: readNumber(row.pressure_grad_y),
		pressure_grad_z: readNumber(row.pressure_grad_z),
		pressure_grad_norm: readNumber(row.pressure_grad_norm),
		divergence: readNumber(row.divergence),
		coherence_mag2: readNumber(row.coherence_mag2),
		guidance_speed: readNumber(row.guidance_speed),
		viscosity_proxy: readNumber(row.viscosity_proxy),
	};
};

const parseCarrier = (raw: unknown): ManifoldCarrierRow | null => {
	if (typeof raw !== "object" || raw === null) {
		return null;
	}

	const row = raw as Record<string, unknown>;

	if (typeof row.symbol !== "string") {
		return null;
	}

	return {
		role: typeof row.role === "string" ? row.role : "symbol",
		symbol: row.symbol,
		x: readNumber(row.x),
		y: readNumber(row.y),
		z: readNumber(row.z),
		cell_x: readNumber(row.cell_x),
		cell_y: readNumber(row.cell_y),
		cell_z: readNumber(row.cell_z),
		amplitude: readNumber(row.amplitude),
		heat: readNumber(row.heat),
		omega: readNumber(row.omega),
		phase: readNumber(row.phase),
		vel_x: readNumber(row.vel_x),
		vel_y: readNumber(row.vel_y),
		vel_z: readNumber(row.vel_z),
	};
};

const parseRhoMatrix = (raw: unknown): number[][] | null => {
	if (!Array.isArray(raw) || raw.length === 0) {
		return null;
	}

	const matrix: number[][] = [];

	for (const row of raw) {
		if (!Array.isArray(row) || row.length === 0) {
			return null;
		}

		const parsedRow: number[] = [];

		for (const value of row) {
			if (typeof value !== "number" || !Number.isFinite(value)) {
				return null;
			}

			parsedRow.push(value);
		}

		matrix.push(parsedRow);
	}

	return matrix;
};

export const parseManifoldSnapshot = (
	raw: unknown,
): ManifoldFieldSnapshot | null => {
	if (typeof raw !== "object" || raw === null) {
		return null;
	}

	const row = raw as Record<string, unknown>;

	if (
		row.type !== "manifold" ||
		!Array.isArray(row.rho) ||
		row.rho.length === 0
	) {
		return null;
	}

	const gridRaw =
		typeof row.grid === "object" && row.grid !== null
			? (row.grid as Record<string, unknown>)
			: {};

	const carriers = Array.isArray(row.carriers)
		? row.carriers
				.map(parseCarrier)
				.filter((carrier): carrier is ManifoldCarrierRow => carrier !== null)
		: [];

	const rho = parseRhoMatrix(row.rho);

	if (rho === null) {
		console.error("manifold snapshot: invalid rho matrix", row.rho);
		return null;
	}

	return {
		type: "manifold",
		ts: typeof row.ts === "string" ? row.ts : "",
		grid: {
			x: readNumber(gridRaw.x),
			y: readNumber(gridRaw.y),
			z: readNumber(gridRaw.z),
			spacing: readNumber(gridRaw.spacing),
		},
		rho,
		reading: parseReading(row.reading),
		carriers,
	};
};

export const formatManifoldReading = (
	frame: ManifoldFieldSnapshot,
): string[] => {
	const reading = frame.reading;
	const whales = frame.carriers.filter(
		(carrier) => carrier.role === "whale",
	).length;
	const symbols = frame.carriers.filter(
		(carrier) => carrier.role === "symbol",
	).length;

	const formatMetric = (value: number): string => {
		if (value === 0) {
			return "0";
		}

		if (Math.abs(value) >= 1e-2) {
			return value.toFixed(4);
		}

		return value.toExponential(2);
	};

	const meanCarrierAmplitude =
		frame.carriers.length === 0
			? 0
			: frame.carriers.reduce(
					(total, carrier) => total + carrier.amplitude,
					0,
				) / frame.carriers.length;

	return [
		`∇p norm ${formatMetric(reading.pressure_grad_norm)}`,
		`mean |Ψ| ${formatMetric(meanCarrierAmplitude)}`,
		`guidance ${formatMetric(reading.guidance_speed)}`,
		`viscosity ${formatMetric(reading.viscosity_proxy)}`,
		`div ${formatMetric(reading.divergence)}`,
		`carriers ${symbols} symbols · ${whales} whales`,
	];
};
