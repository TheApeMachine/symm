import { sliceShader, volumeShader } from "./field-shaders";
import { packFluidFieldTextures } from "./field-textures";
import {
	createUniformBuffer,
	createVertexBuffer,
	createVolumeTexture,
	FRAME_UNIFORM_FLOATS,
	type FluidGPU,
	writeTexture3D,
} from "./gpu";
import type { FluidFields, FluidGrid } from "./wire";

export type FluidFieldOptions = {
	gas: boolean;
	wave: boolean;
	volume: boolean;
	slices: boolean;
	exposure: number;
};

const cubeTriangles = () => {
	const quads: Array<Array<[number, number, number]>> = [
		[
			[0, 0, 0],
			[0, 1, 0],
			[1, 1, 0],
			[1, 0, 0],
		],
		[
			[0, 0, 1],
			[1, 0, 1],
			[1, 1, 1],
			[0, 1, 1],
		],
		[
			[0, 0, 0],
			[1, 0, 0],
			[1, 0, 1],
			[0, 0, 1],
		],
		[
			[0, 1, 0],
			[0, 1, 1],
			[1, 1, 1],
			[1, 1, 0],
		],
		[
			[0, 0, 0],
			[0, 0, 1],
			[0, 1, 1],
			[0, 1, 0],
		],
		[
			[1, 0, 0],
			[1, 1, 0],
			[1, 1, 1],
			[1, 0, 1],
		],
	];
	const triangles = new Float32Array(36 * 3);
	let offset = 0;

	for (const quad of quads) {
		for (const corner of [0, 1, 2, 0, 2, 3]) {
			triangles.set(quad[corner]!, offset);
			offset += 3;
		}
	}

	return triangles;
};

const sliceTriangles = (x: number, y: number, z: number) => {
	const triangles = new Float32Array(18 * 3);
	const planes: Array<Array<[number, number, number]>> = [
		[
			[x, 0, 0],
			[x, 1, 0],
			[x, 1, 1],
			[x, 0, 1],
		],
		[
			[0, y, 0],
			[0, y, 1],
			[1, y, 1],
			[1, y, 0],
		],
		[
			[0, 0, z],
			[1, 0, z],
			[1, 1, z],
			[0, 1, z],
		],
	];
	let offset = 0;

	for (const quad of planes) {
		for (const corner of [0, 1, 2, 0, 2, 3]) {
			triangles.set(quad[corner]!, offset);
			offset += 3;
		}
	}

	return triangles;
};

const fieldBindGroupLayout = (device: GPUDevice) =>
	device.createBindGroupLayout({
		entries: [
			{
				binding: 0,
				visibility: GPUShaderStage.VERTEX | GPUShaderStage.FRAGMENT,
				buffer: { type: "uniform" },
			},
			{ binding: 1, visibility: GPUShaderStage.FRAGMENT, sampler: {} },
			{ binding: 2, visibility: GPUShaderStage.FRAGMENT, texture: { viewDimension: "3d" } },
			{ binding: 3, visibility: GPUShaderStage.FRAGMENT, texture: { viewDimension: "3d" } },
			{ binding: 4, visibility: GPUShaderStage.FRAGMENT, texture: { viewDimension: "3d" } },
			{ binding: 5, visibility: GPUShaderStage.FRAGMENT, texture: { viewDimension: "3d" } },
		],
	});

const pipeline = (
	gpu: FluidGPU,
	layout: GPUBindGroupLayout,
	code: string,
) =>
	gpu.device.createRenderPipeline({
		layout: gpu.device.createPipelineLayout({ bindGroupLayouts: [layout] }),
		vertex: {
			module: gpu.device.createShaderModule({ code }),
			entryPoint: "vs_main",
			buffers: [
				{
					arrayStride: 12,
					attributes: [{ shaderLocation: 0, offset: 0, format: "float32x3" }],
				},
			],
		},
		fragment: {
			module: gpu.device.createShaderModule({ code }),
			entryPoint: "fs_main",
			targets: [
				{
					format: gpu.format,
					blend: {
						color: {
							srcFactor: "src-alpha",
							dstFactor: "one-minus-src-alpha",
							operation: "add",
						},
						alpha: {
							srcFactor: "one",
							dstFactor: "one-minus-src-alpha",
							operation: "add",
						},
					},
				},
			],
		},
		primitive: { topology: "triangle-list", cullMode: "none" },
		depthStencil: {
			format: "depth24plus",
			depthWriteEnabled: false,
			depthCompare: "less",
		},
	});

/*
FluidFieldView renders the resident Eulerian gas and complex wave arrays as a
raymarched unit volume and three independently movable diagnostic slices.
*/
export class FluidFieldView {
	private textures: {
		momRho: GPUTexture;
		internalEnergy: GPUTexture;
		waveReal: GPUTexture;
		waveImaginary: GPUTexture;
	} | null = null;
	private bindGroup: GPUBindGroup | null = null;
	private grid: FluidGrid | null = null;
	private options: FluidFieldOptions = {
		gas: true,
		wave: true,
		volume: true,
		slices: false,
		exposure: 1.5,
	};
	private slices = { x: 0.5, y: 0.5, z: 0.5 };
	private densityScale = 1;
	private momentumScale = 1;
	private energyScale = 1;
	private waveScale = 1;
	private readonly bindLayout: GPUBindGroupLayout;
	private readonly volumePipeline: GPURenderPipeline;
	private readonly slicePipeline: GPURenderPipeline;
	private readonly sampler: GPUSampler;
	private readonly cubeBuffer: GPUBuffer;
	private readonly sliceBuffer: GPUBuffer;
	readonly uniformBuffer: GPUBuffer;

	constructor(private readonly gpu: FluidGPU) {
		this.bindLayout = fieldBindGroupLayout(gpu.device);
		this.volumePipeline = pipeline(gpu, this.bindLayout, volumeShader);
		this.slicePipeline = pipeline(gpu, this.bindLayout, sliceShader);
		this.sampler = gpu.device.createSampler({
			magFilter: gpu.sampleFilter,
			minFilter: gpu.sampleFilter,
			addressModeU: "clamp-to-edge",
			addressModeV: "clamp-to-edge",
			addressModeW: "clamp-to-edge",
		});
		this.cubeBuffer = createVertexBuffer(gpu.device, cubeTriangles());
		this.sliceBuffer = createVertexBuffer(gpu.device, sliceTriangles(0.5, 0.5, 0.5));
		this.uniformBuffer = createUniformBuffer(gpu.device, FRAME_UNIFORM_FLOATS);
	}

	update(fields: FluidFields) {
		const packed = packFluidFieldTextures(fields);

		if (this.textures === null) {
			this.textures = {
				momRho: createVolumeTexture(this.gpu.device, "rgba32float", packed.momRho.extent),
				internalEnergy: createVolumeTexture(
					this.gpu.device,
					"r32float",
					packed.internalEnergy.extent,
				),
				waveReal: createVolumeTexture(this.gpu.device, "r32float", packed.waveReal.extent),
				waveImaginary: createVolumeTexture(
					this.gpu.device,
					"r32float",
					packed.waveImaginary.extent,
				),
			};
			this.bindGroup = this.gpu.device.createBindGroup({
				layout: this.bindLayout,
				entries: [
					{ binding: 0, resource: { buffer: this.uniformBuffer } },
					{ binding: 1, resource: this.sampler },
					{ binding: 2, resource: this.textures.momRho.createView() },
					{ binding: 3, resource: this.textures.internalEnergy.createView() },
					{ binding: 4, resource: this.textures.waveReal.createView() },
					{ binding: 5, resource: this.textures.waveImaginary.createView() },
				],
			});
		}

		if (
			this.grid !== null &&
			(this.grid.x !== fields.grid.x ||
				this.grid.y !== fields.grid.y ||
				this.grid.z !== fields.grid.z)
		) {
			throw new Error("fluid grid dimensions changed during a resident session");
		}

		writeTexture3D(this.gpu.device, this.textures.momRho, packed.momRho);
		writeTexture3D(this.gpu.device, this.textures.internalEnergy, packed.internalEnergy);
		writeTexture3D(this.gpu.device, this.textures.waveReal, packed.waveReal);
		writeTexture3D(this.gpu.device, this.textures.waveImaginary, packed.waveImaginary);
		this.grid = fields.grid;
		this.densityScale = fields.densityScale;
		this.momentumScale = fields.momentumScale;
		this.energyScale = fields.energyScale;
		this.waveScale = fields.waveScale;
	}

	setOptions(options: FluidFieldOptions) {
		this.options = options;
	}

	setSlices(x: number, y: number, z: number) {
		this.slices = { x, y, z };
		this.gpu.device.queue.writeBuffer(
			this.sliceBuffer,
			0,
			sliceTriangles(x, y, z),
		);
	}

	writeFrame(
		viewProj: Float32Array,
		invViewProj: Float32Array,
		cameraPos: [number, number, number],
	) {
		const data = new Float32Array(FRAME_UNIFORM_FLOATS);
		data.set(viewProj, 0);
		data.set(invViewProj, 16);
		data[32] = cameraPos[0];
		data[33] = cameraPos[1];
		data[34] = cameraPos[2];
		data[35] = this.options.exposure;
		data[36] = this.grid?.x ?? 1;
		data[37] = this.grid?.y ?? 1;
		data[38] = this.grid?.z ?? 1;
		data[39] = this.densityScale;
		data[40] = this.momentumScale;
		data[41] = this.energyScale;
		data[42] = this.waveScale;
		data[43] = this.options.gas ? 1 : 0;
		data[44] = this.options.wave ? 1 : 0;
		data[45] = this.slices.x;
		data[46] = this.slices.y;
		data[47] = this.slices.z;
		this.gpu.device.queue.writeBuffer(this.uniformBuffer, 0, data);
	}

	encode(pass: GPURenderPassEncoder) {
		if (this.bindGroup === null) {
			return;
		}

		pass.setBindGroup(0, this.bindGroup);

		if (this.options.volume) {
			pass.setPipeline(this.volumePipeline);
			pass.setVertexBuffer(0, this.cubeBuffer);
			pass.draw(36);
		}

		if (this.options.slices) {
			pass.setPipeline(this.slicePipeline);
			pass.setVertexBuffer(0, this.sliceBuffer);
			pass.draw(18);
		}
	}

	dispose() {
		this.textures?.momRho.destroy();
		this.textures?.internalEnergy.destroy();
		this.textures?.waveReal.destroy();
		this.textures?.waveImaginary.destroy();
		this.cubeBuffer.destroy();
		this.sliceBuffer.destroy();
		this.uniformBuffer.destroy();
	}
}
