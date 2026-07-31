/*
drawFluidDisplay blits the backend-composited GPU RGBA texture. The frontend does
not synthesize a fallback fluid field from scalar lattices.
*/
export const drawFluidDisplay = (
	canvas: HTMLCanvasElement,
	width: number,
	height: number,
): boolean => {
	const frame = null; // TODO: get the latest frame from the store or context

	if (frame === null || width <= 0 || height <= 0) {
		return false;
	}

	const context = canvas.getContext("2d");

	if (context === null) {
		return false;
	}

	const dpr = Math.max(1, window.devicePixelRatio || 1);
	const pixelWidth = Math.max(1, Math.floor(width * dpr));
	const pixelHeight = Math.max(1, Math.floor(height * dpr));

	if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
		canvas.width = pixelWidth;
		canvas.height = pixelHeight;
	}

	const tile = document.createElement("canvas");
	// tile.width = (frame.width ?? 0);
	// tile.height = frame.height ?? 0;
	const tileContext = tile.getContext("2d");

	if (tileContext === null) {
		return false;
	}

	// const image = tileContext.createImageData(frame.width, frame.height);
	// image.data.set(frame.rgba);
	// tileContext.putImageData(image, 0, 0);
	context.imageSmoothingEnabled = true;
	context.imageSmoothingQuality = "high";
	context.clearRect(0, 0, pixelWidth, pixelHeight);
	context.drawImage(tile, 0, 0, pixelWidth, pixelHeight);
	return true;
};
