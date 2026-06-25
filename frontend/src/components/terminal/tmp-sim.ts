export const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

export const rnd = (min: number, max: number): number =>
	min + Math.random() * (max - min);

export type FluidBlob = {
	x: number;
	y: number;
	vx: number;
	vy: number;
	a: number;
	r: number;
};

export type WhaleCarrier = {
	x: number;
	y: number;
	vx: number;
	vy: number;
};

export type FluidSim = {
	blobs: FluidBlob[];
	whales: WhaleCarrier[];
	peak: number;
};

export type HawkesSim = {
	mu: number;
	alpha: number;
	beta: number;
	lam: number;
	events: number[];
	buf: number[];
};

export type PredBuffer = {
	actual: number[];
	pred: number[];
};

export type ManifoldPoint = {
	symbol: string;
	lx: number;
	ly: number;
	vol: number;
	cluster: number;
};

export const createFluidSim = (): FluidSim => ({
	blobs: Array.from({ length: 5 }, () => ({
		x: Math.random(),
		y: Math.random(),
		vx: rnd(-0.06, 0.06),
		vy: rnd(-0.05, 0.05),
		a: rnd(0.6, 1),
		r: rnd(0.12, 0.26),
	})),
	whales: Array.from({ length: 3 }, () => ({
		x: Math.random(),
		y: Math.random(),
		vx: rnd(-0.04, 0.04),
		vy: rnd(-0.03, 0.03),
	})),
	peak: 0,
});

export const advanceFluidSim = (sim: FluidSim): void => {
	for (const blob of sim.blobs) {
		blob.x += blob.vx * 0.016;
		blob.y += blob.vy * 0.016;

		if (blob.x < 0 || blob.x > 1) {
			blob.vx *= -1;
		}

		if (blob.y < 0 || blob.y > 1) {
			blob.vy *= -1;
		}

		blob.x = clamp(blob.x, 0, 1);
		blob.y = clamp(blob.y, 0, 1);
	}

	for (const whale of sim.whales) {
		whale.x += whale.vx * 0.016;
		whale.y += whale.vy * 0.016;

		if (whale.x < 0.05 || whale.x > 0.95) {
			whale.vx *= -1;
		}

		if (whale.y < 0.05 || whale.y > 0.95) {
			whale.vy *= -1;
		}

		whale.x = clamp(whale.x, 0.05, 0.95);
		whale.y = clamp(whale.y, 0.05, 0.95);
	}
};

export const createHawkesSim = (): HawkesSim => ({
	mu: 0.2,
	alpha: 0.68,
	beta: 1.25,
	lam: 0.2,
	events: [],
	buf: [],
});

export const stepHawkesSim = (hawkes: HawkesSim): void => {
	const dt = 0.09;
	hawkes.lam =
		hawkes.mu + (hawkes.lam - hawkes.mu) * Math.exp(-hawkes.beta * dt);

	if (Math.random() < clamp(hawkes.lam * dt * 1.5, 0, 0.9)) {
		hawkes.lam += hawkes.alpha;
		hawkes.events.push(performance.now());
	}

	if (hawkes.events.length > 80) {
		hawkes.events.shift();
	}

	hawkes.buf.push(hawkes.lam);

	if (hawkes.buf.length > 220) {
		hawkes.buf.shift();
	}
};

export const createPredBuffer = (): PredBuffer => ({
	actual: [],
	pred: [],
});

export const stepPredBuffer = (buffer: PredBuffer, trend = 0.5): void => {
	const last = buffer.actual.at(-1) ?? 0.5;
	const next = clamp(
		last + rnd(-0.06, 0.06) + (trend - 0.5) * 0.02,
		0.05,
		0.95,
	);

	buffer.actual.push(next);
	buffer.pred.push(clamp(next + rnd(-0.07, 0.07), 0.05, 0.95));

	if (buffer.actual.length > 130) {
		buffer.actual.shift();
		buffer.pred.shift();
	}
};

export const manifoldFromFluidFrame = (
	frame: Record<string, unknown> | null,
	_focusSymbol: string,
): ManifoldPoint[] => {
	const symbols = frame?.symbols;

	if (!Array.isArray(symbols) || symbols.length === 0) {
		return [];
	}

	return symbols.slice(0, 48).map((row, index) => {
		const record =
			typeof row === "object" && row !== null
				? (row as Record<string, unknown>)
				: {};
		const symbol =
			typeof record.symbol === "string" ? record.symbol : `S${index}`;
		const change =
			typeof record.change_pct === "number" ? record.change_pct : 0;
		const volume =
			typeof record.volume === "number"
				? record.volume
				: typeof record.vol === "number"
					? record.vol
					: 1;

		return {
			symbol: symbol.split("/")[0] ?? symbol,
			lx: clamp(Math.sin(index * 0.7 + volume) * 0.9, -1, 1),
			ly: clamp(Math.cos(index * 0.5 + change) * 0.9, -1, 1),
			vol: clamp(volume / 1_000_000, 0.2, 1),
			cluster: index % 4,
		};
	});
};

export const hawkesFromReading = (
	surprise: number,
	samples: number,
): Partial<HawkesSim> => {
	const alpha = clamp(0.4 + surprise * 0.15, 0.4, 1.05);
	const beta = 1.25;
	const mu = 0.2;
	const lam = mu + alpha * clamp(samples / 120, 0, 1) * 0.4;

	return { mu, alpha, beta, lam };
};
