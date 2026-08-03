import { latestDisplay } from "#/providers/manifold-binary";

/*
The scratch tile is created on first draw rather than at import, because a
module that touches the document as a side effect of being imported cannot load
without a DOM — including in any test that reaches it transitively through the
paint registry.
*/
let tile: HTMLCanvasElement | null = null;
let tileContext: CanvasRenderingContext2D | null = null;

/*
drawFluidDisplay blits the backend-composited GPU RGBA texture. The frontend does
not synthesize a fallback fluid field from scalar lattices.
*/
export const drawFluidDisplay = (
	canvas: HTMLCanvasElement,
	width: number,
	height: number,
): boolean => {
	const frame = latestDisplay();

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

	if (tile === null) {
		tile = document.createElement("canvas");
		tileContext = tile.getContext("2d");
	}

	if (tileContext === null) {
		return false;
	}

	if (tile.width !== frame.width || tile.height !== frame.height) {
		tile.width = frame.width;
		tile.height = frame.height;
	}

	const image = tileContext.createImageData(frame.width, frame.height);
	image.data.set(frame.rgba);
	tileContext.putImageData(image, 0, 0);
	context.imageSmoothingEnabled = true;
	context.imageSmoothingQuality = "high";
	context.clearRect(0, 0, pixelWidth, pixelHeight);
	context.drawImage(tile, 0, 0, pixelWidth, pixelHeight);
	return true;
};
