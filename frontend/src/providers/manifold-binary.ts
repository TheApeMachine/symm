/*
manifold-binary decodes backend-composited SMF1 display frames. The GPU has
already shaded the image, so the browser only retains and blits RGBA8 pixels.
*/

import { createStore } from "@tanstack/store";

export type DisplayFrame = {
	kind: "display";
	symbol: string;
	width: number;
	height: number;
	rgba: Uint8ClampedArray;
};

const displayStore = createStore<DisplayFrame | null>(null);

export const parseManifoldBinary = (
	buffer: ArrayBuffer,
): DisplayFrame | null => {
	if (buffer.byteLength < 26) {
		return null;
	}

	const data = new DataView(buffer);

	if (
		data.getUint8(0) !== 0x53 ||
		data.getUint8(1) !== 0x4d ||
		data.getUint8(2) !== 0x46 ||
		data.getUint8(3) !== 0x31 ||
		data.getUint8(4) !== 5
	) {
		return null;
	}

	const width = data.getUint16(5, true);
	const height = data.getUint16(7, true);
	const symbolLength = data.getUint8(25);
	const pixelOffset = 26 + symbolLength;
	const pixelBytes = width * height * 4;

	if (
		width < 1 ||
		height < 1 ||
		buffer.byteLength !== pixelOffset + pixelBytes
	) {
		return null;
	}

	return {
		kind: "display",
		symbol: new TextDecoder().decode(
			new Uint8Array(buffer, 26, symbolLength),
		),
		width,
		height,
		rgba: new Uint8ClampedArray(
			buffer.slice(pixelOffset, pixelOffset + pixelBytes),
		),
	};
};

export const retainManifoldBinary = (buffer: ArrayBuffer): boolean => {
	const frame = parseManifoldBinary(buffer);

	if (frame === null) {
		return false;
	}

	displayStore.setState(() => frame);
	return true;
};

export const latestDisplay = (): DisplayFrame | null => displayStore.state;

export const clearManifoldBinary = () => {
	displayStore.setState(() => null);
};
