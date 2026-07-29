/*
manifold-binary retains the raw RGBA display arriving on the dedicated manifold
socket. Width and height travel on the normal JSON manifold lane so the image
bytes stay off the main websocket path.
*/

export type DisplayFrame = {
	kind: "display";
	symbol: string;
	width: number;
	height: number;
	rgba: Uint8ClampedArray;
};

let display: DisplayFrame | null = null;
let meta: Pick<DisplayFrame, "symbol" | "width" | "height"> | null = null;

/*
retainManifoldMeta keeps the latest symbol and dimensions needed to interpret
the raw RGBA frame from the manifold socket.
 */
export const retainManifoldMeta = (value: unknown) => {
	const rows = Array.isArray(value) ? value : value != null ? [value] : [];
	const latest = rows.at(-1);

	if (latest === undefined || latest === null || typeof latest !== "object") {
		return;
	}

	const record = latest as Record<string, unknown>;
	const symbol = typeof record.symbol === "string" ? record.symbol : "shared";
	const width = Number(record.width);
	const height = Number(record.height);

	if (!Number.isFinite(width) || !Number.isFinite(height) || width < 1 || height < 1) {
		return;
	}

	meta = {
		symbol,
		width,
		height,
	};
};

export const retainManifoldBinary = (buffer: ArrayBuffer): "display" | null => {
	if (meta === null) {
		return null;
	}

	if (buffer.byteLength !== meta.width * meta.height * 4) {
		return null;
	}

	display = {
		kind: "display",
		symbol: meta.symbol,
		width: meta.width,
		height: meta.height,
		rgba: new Uint8ClampedArray(buffer.slice(0)),
	};

	return "display";
};

export const latestDisplay = (): DisplayFrame | null => display;

export const clearManifoldBinary = () => {
	display = null;
	meta = null;
};
