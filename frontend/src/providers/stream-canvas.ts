import type {
	StreamMetric,
	StreamPalette,
	StreamRegistration,
	StreamWorkerMessage,
} from "./stream-protocol";

type StreamDataset = DOMStringMap & {
	streamFilter?: string;
	streamValue?: string;
	streamBaseline?: string;
	streamDecay?: string;
	streamRug?: string;
	appendLimit?: string;
};

type CanvasRegistration = {
	id: number;
	canvas: HTMLCanvasElement;
	offscreen: OffscreenCanvas;
	metric: StreamMetric;
	observer: ResizeObserver | null;
	owner: Worker | null;
	disposeTimer: ReturnType<typeof setTimeout> | null;
};

let worker: Worker | null = null;
let nextID = 1;
const registrations = new Map<number, CanvasRegistration>();
const byCanvas = new WeakMap<HTMLCanvasElement, CanvasRegistration>();

const rgba = (value: string): [number, number, number, number] => {
	const match = /^#([\da-f]{2})([\da-f]{2})([\da-f]{2})$/i.exec(value);

	if (match === null) {
		throw new Error(
			`stream renderer requires a resolved hexadecimal color: ${value}`,
		);
	}

	return [
		Number.parseInt(match[1], 16) / 255,
		Number.parseInt(match[2], 16) / 255,
		Number.parseInt(match[3], 16) / 255,
		1,
	];
};

const palette = (canvas: HTMLCanvasElement): StreamPalette => {
	const styles = getComputedStyle(canvas);

	return {
		background: rgba(styles.getPropertyValue("--sunken").trim()),
		grid: rgba(styles.getPropertyValue("--line").trim()),
		accent: rgba(styles.getPropertyValue("--acc").trim()),
	};
};

const metricName = (path: string | undefined, required: boolean): string | undefined => {
	if (path === undefined && required) {
		throw new Error("stream renderer requires a metrics.<name>.raw value path");
	}

	if (path === undefined) {
		return undefined;
	}

	const match = /^metrics\.(.+)\.raw$/.exec(path);

	if (match === null) {
		throw new Error(`stream metric path must be metrics.<name>.raw: ${path}`);
	}

	const name = match[1];

	if (name === undefined) {
		throw new Error(`stream metric path has no name: ${path}`);
	}

	return name;
};

const metric = (dataset: StreamDataset): StreamMetric => {
	const filters = Object.fromEntries(
		(dataset.streamFilter ?? "").split(",").map((condition) => {
			const [name, value] = condition.split("=");

			if (!name || value === undefined) {
				throw new Error(`stream filter needs name=value: ${condition}`);
			}

			return [name.trim(), value.trim()];
		}),
	);

	if (!filters.source || !filters.symbol) {
		throw new Error("stream renderer requires source and symbol filters");
	}

	const value = metricName(dataset.streamValue, true);

	if (value === undefined) {
		throw new Error("stream renderer requires a value metric");
	}

	return {
		source: filters.source,
		symbol: filters.symbol,
		value,
		baseline: metricName(dataset.streamBaseline, false),
		decay: metricName(dataset.streamDecay, false),
	};
};

const dimensions = (canvas: HTMLCanvasElement) => ({
	width: Math.max(1, canvas.clientWidth),
	height: Math.max(1, canvas.clientHeight),
	pixelRatio: window.devicePixelRatio,
});

const sendRegistration = (registration: CanvasRegistration) => {
	if (worker === null) {
		return;
	}

	if (registration.owner === worker) {
		return;
	}

	if (registration.owner !== null) {
		throw new Error(
			"stream canvas ownership requires a canvas remount before replacing its worker",
		);
	}

	const capacity = Number.parseInt(
		registration.canvas.dataset.appendLimit ?? "",
		10,
	);

	if (!Number.isInteger(capacity) || capacity < 2) {
		throw new Error("stream renderer requires data-append-limit >= 2");
	}

	const message: StreamRegistration = {
		id: registration.id,
		canvas: registration.offscreen,
		...dimensions(registration.canvas),
		capacity,
		metric: registration.metric,
		palette: palette(registration.canvas),
		rug: registration.canvas.dataset.streamRug !== undefined,
	};
	worker.postMessage(
		{
			type: "REGISTER_STREAM",
			registration: message,
		} satisfies StreamWorkerMessage,
		[registration.offscreen],
	);
	registration.owner = worker;
};

export const bindStreamWorker = (next: Worker | null) => {
	if (worker === next) {
		return;
	}

	worker = next;

	if (worker !== null) {
		for (const registration of registrations.values()) {
			sendRegistration(registration);
		}
	}
};

export const registerStreamCanvas = (
	canvas: HTMLCanvasElement,
	dataset: StreamDataset,
) => {
	const existing = byCanvas.get(canvas);
	const nextMetric = metric(dataset);

	if (existing !== undefined) {
		if (existing.disposeTimer !== null) {
			clearTimeout(existing.disposeTimer);
			existing.disposeTimer = null;
		}

		existing.metric = nextMetric;
		worker?.postMessage({
			type: "UPDATE_STREAM",
			id: existing.id,
			metric: nextMetric,
		} satisfies StreamWorkerMessage);
		return;
	}

	const offscreen = canvas.transferControlToOffscreen();
	const registration: CanvasRegistration = {
		id: nextID++,
		canvas,
		offscreen,
		metric: nextMetric,
		observer: null,
		owner: null,
		disposeTimer: null,
	};
	registration.observer = new ResizeObserver(() => {
		worker?.postMessage({
			type: "RESIZE_STREAM",
			id: registration.id,
			...dimensions(canvas),
		} satisfies StreamWorkerMessage);
	});
	registration.observer.observe(canvas);
	registrations.set(registration.id, registration);
	byCanvas.set(canvas, registration);
	sendRegistration(registration);
};

export const unregisterStreamCanvas = (canvas: HTMLCanvasElement) => {
	const registration = byCanvas.get(canvas);

	if (registration === undefined || registration.disposeTimer !== null) {
		return;
	}

	registration.disposeTimer = setTimeout(() => {
		registration.observer?.disconnect();
		registrations.delete(registration.id);
		byCanvas.delete(canvas);
		registration.owner?.postMessage({
			type: "UNREGISTER_STREAM",
			id: registration.id,
		} satisfies StreamWorkerMessage);
	}, 0);
};