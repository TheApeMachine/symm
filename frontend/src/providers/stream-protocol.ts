/*
stream-protocol is the message contract between the main-thread canvas
registration (stream-canvas.ts) and the worker-side GPU renderer
(gpu-stream.ts): what one streamed metric selector is, what one canvas
registration carries across the OffscreenCanvas transfer, and the messages
the main thread posts to keep a registration in sync.
*/

/*
StreamMetric selects one series: value is required, baseline/decay are
optional companion series rendered alongside it (e.g. a mean line, a decay
envelope). source/symbol identify which measurement stream to read from.
*/
export type StreamMetric = {
	source: string;
	symbol: string;
	value: string;
	baseline?: string;
	decay?: string;
};

/*
StreamPalette is resolved once on the main thread (getComputedStyle needs a
DOM element) and sent across so the worker never touches CSS custom
properties. Each color is a straight RGBA tuple in [0, 1].
*/
export type StreamPalette = {
	background: [number, number, number, number];
	grid: [number, number, number, number];
	accent: [number, number, number, number];
};

/*
StreamRegistration is one canvas's full registration payload: the transferred
OffscreenCanvas plus everything GPUStreamRenderer needs to size, style, and
begin sampling immediately. width/height/pixelRatio are mutated in place by
later RESIZE_STREAM handling on the worker side.
*/
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

export type StreamRegisterMessage = {
	type: "REGISTER_STREAM";
	registration: StreamRegistration;
};

export type StreamUpdateMessage = {
	type: "UPDATE_STREAM";
	id: number;
	metric: StreamMetric;
};

export type StreamResizeMessage = {
	type: "RESIZE_STREAM";
	id: number;
	width: number;
	height: number;
	pixelRatio: number;
};

export type StreamUnregisterMessage = {
	type: "UNREGISTER_STREAM";
	id: number;
};

export type StreamWorkerMessage =
	| StreamRegisterMessage
	| StreamUpdateMessage
	| StreamResizeMessage
	| StreamUnregisterMessage;
