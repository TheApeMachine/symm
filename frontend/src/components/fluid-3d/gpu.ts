export const FRAME_UNIFORM_FLOATS = 48;
export const PARTICLE_UNIFORM_FLOATS = 32;
export const CLEAR_COLOR = { r: 14 / 255, g: 12 / 255, b: 10 / 255, a: 1 };

export type FluidGPU = {
	device: GPUDevice;
	context: GPUCanvasContext;
	format: GPUTextureFormat;
	sampleFilter: GPUFilterMode;
};

export const connectFluidGPU = async (
	canvas: HTMLCanvasElement,
): Promise<FluidGPU> => {
	const gpu = navigator.gpu;

	if (gpu === undefined) {
		throw new Error("WebGPU is required for the fluid manifold inspector");
	}

	const adapter = await gpu.requestAdapter();

	if (adapter === null) {
		throw new Error("WebGPU adapter is unavailable");
	}

	const filterable = adapter.features.has("float32-filterable");
	const device = await adapter.requestDevice({
		requiredFeatures: filterable ? ["float32-filterable"] : [],
	});
	const context = canvas.getContext("webgpu");

	if (context === null) {
		throw new Error("canvas rejected WebGPU");
	}

	const format = gpu.getPreferredCanvasFormat();
	context.configure({ device, format, alphaMode: "opaque" });
	return {
		device,
		context,
		format,
		sampleFilter: filterable ? "linear" : "nearest",
	};
};

export const createUniformBuffer = (device: GPUDevice, floats: number) =>
	device.createBuffer({
		size: floats * Float32Array.BYTES_PER_ELEMENT,
		usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
	});

export const createVertexBuffer = (device: GPUDevice, data: Float32Array) => {
	const buffer = device.createBuffer({
		size: data.byteLength,
		usage: GPUBufferUsage.VERTEX | GPUBufferUsage.COPY_DST,
	});
	device.queue.writeBuffer(buffer, 0, data);
	return buffer;
};

export const writeTexture3D = (
	device: GPUDevice,
	texture: GPUTexture,
	packed: {
		data: Float32Array;
		bytesPerRow: number;
		rowsPerImage: number;
		extent: { width: number; height: number; depth: number };
	},
) => {
	device.queue.writeTexture(
		{ texture },
		packed.data,
		{
			bytesPerRow: packed.bytesPerRow,
			rowsPerImage: packed.rowsPerImage,
		},
		{
			width: packed.extent.width,
			height: packed.extent.height,
			depthOrArrayLayers: packed.extent.depth,
		},
	);
};

export const createVolumeTexture = (
	device: GPUDevice,
	format: GPUTextureFormat,
	extent: { width: number; height: number; depth: number },
) =>
	device.createTexture({
		format,
		dimension: "3d",
		size: {
			width: extent.width,
			height: extent.height,
			depthOrArrayLayers: extent.depth,
		},
		usage: GPUTextureUsage.TEXTURE_BINDING | GPUTextureUsage.COPY_DST,
	});
