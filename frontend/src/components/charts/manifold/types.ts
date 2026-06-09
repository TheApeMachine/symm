export type ManifoldReadingRow = {
	pressure_grad_x: number;
	pressure_grad_y: number;
	pressure_grad_z: number;
	pressure_grad_norm: number;
	divergence: number;
	coherence_mag2: number;
	guidance_speed: number;
	viscosity_proxy: number;
};

export type ManifoldCarrierRow = {
	role: "symbol" | "whale" | string;
	symbol: string;
	x: number;
	y: number;
	z: number;
	cell_x: number;
	cell_y: number;
	cell_z: number;
	amplitude: number;
	heat: number;
	omega: number;
	phase: number;
	vel_x: number;
	vel_y: number;
	vel_z: number;
};

export type ManifoldFieldSnapshot = {
	type: "manifold";
	ts: string;
	grid: {
		x: number;
		y: number;
		z: number;
		spacing: number;
	};
	rho: number[][];
	reading: ManifoldReadingRow;
	carriers: ManifoldCarrierRow[];
};
