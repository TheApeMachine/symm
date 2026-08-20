export type StreamMetric = {
	source: string;
	symbol: string;
	value: string;
	baseline?: string;
	decay?: string;
};

export type StreamPalette = {
	background: [number, number, number, number];
	grid: [number, number, number, number];
	accent: [number, number, number, number];
};

export type StreamRegistration = {
	id: number;
	canvas: OffscreenCanvas;
	width: number;
	height: number;
	pixelRatio: number;
	capacity: number;
	metric: StreamMetric;
	palette: StreamPalette;
	rug: boolean;
};

export type StreamWorkerMessage =
	| { type: "REGISTER_STREAM"; registration: StreamRegistration }
	| {
			type: "RESIZE_STREAM";
			id: number;
			width: number;
			height: number;
			pixelRatio: number;
	  }
	| { type: "UPDATE_STREAM"; id: number; metric: StreamMetric }
	| { type: "UNREGISTER_STREAM"; id: number };
