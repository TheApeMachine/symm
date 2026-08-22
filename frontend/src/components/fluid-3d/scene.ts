import { OrbitCamera } from "./camera";
import { type FluidFieldOptions, FluidFieldView } from "./fields";
import { lineShader } from "./field-shaders";
import { CLEAR_COLOR, connectFluidGPU, createVertexBuffer, type FluidGPU } from "./gpu";
import { FluidParticles } from "./particles";
import type { FluidFields, FluidParticle, FluidParticleFrame } from "./wire";

export type FluidSceneOptions = FluidFieldOptions & {
	particles: boolean;
};

const boundaryLines = () => {
	const corners: Array<[number, number, number]> = [
		[0, 0, 0],
		[1, 0, 0],
		[1, 1, 0],
		[0, 1, 0],
		[0, 0, 1],
		[1, 0, 1],
		[1, 1, 1],
		[0, 1, 1],
	];
	const edges = [
		[0, 1],
		[1, 2],
		[2, 3],
		[3, 0],
		[4, 5],
		[5, 6],
		[6, 7],
		[7, 4],
		[0, 4],
		[1, 5],
		[2, 6],
		[3, 7],
	];
	const lines = new Float32Array(edges.length * 2 * 3);
	let offset = 0;

	for (const [start, end] of edges) {
		lines.set(corners[start!]!, offset);
		offset += 3;
		lines.set(corners[end!]!, offset);
		offset += 3;
	}

	return lines;
};

/*
FluidScene owns the WebGPU resources for the interactive inspector. Incoming
domain values update GPU buffers and textures directly through the two views.
*/
export class FluidScene {
	private readonly canvas: HTMLCanvasElement;
	private readonly camera = new OrbitCamera();
	private readonly resizeObserver: ResizeObserver;
	private gpu: FluidGPU | null = null;
	private fields: FluidFieldView | null = null;
	private particles: FluidParticles | null = null;
	private boundaryBuffer: GPUBuffer | null = null;
	private boundaryPipeline: GPURenderPipeline | null = null;
	private boundaryBindGroup: GPUBindGroup | null = null;
	private depthTexture: GPUTexture | null = null;
	private animationFrame = 0;
	private disposed = false;
	private options: FluidSceneOptions = {
		particles: true,
		gas: true,
		wave: true,
		volume: true,
		slices: false,
		exposure: 1.5,
	};
	private queuedFields: FluidFields | null = null;
	private queuedParticles: FluidParticleFrame | null = null;
	private pointerDownX = 0;
	private pointerDownY = 0;

	constructor(
		private readonly container: HTMLElement,
		private readonly onSelect: (particle: FluidParticle | null) => void,
		private readonly onError?: (error: Error) => void,
	) {
		if (navigator.gpu === undefined) {
			throw new Error("WebGPU is required for the fluid manifold inspector");
		}

		this.canvas = document.createElement("canvas");
		this.canvas.className = "block size-full touch-none";
		this.container.append(this.canvas);
		this.camera.attach(this.canvas, this.invalidate);
		this.canvas.addEventListener("pointerdown", this.onPointerDown);
		this.canvas.addEventListener("pointerup", this.pick);
		this.resizeObserver = new ResizeObserver(this.resize);
		this.resizeObserver.observe(this.container);
		void this.initialize();
	}

	updateFields(fields: FluidFields) {
		this.queuedFields = fields;
		this.fields?.update(fields);
		this.particles?.setGridSpacing(fields.grid.spacing);
		this.invalidate();
	}

	updateParticles(particles: FluidParticleFrame) {
		this.queuedParticles = particles;
		this.particles?.update(particles);
		this.invalidate();
	}

	setOptions(options: FluidSceneOptions) {
		this.options = options;
		this.fields?.setOptions(options);

		if (this.particles !== null) {
			this.particles.visible = options.particles;
		}

		this.invalidate();
	}

	setSlices(x: number, y: number, z: number) {
		this.fields?.setSlices(x, y, z);
		this.invalidate();
	}

	dispose() {
		this.disposed = true;
		cancelAnimationFrame(this.animationFrame);
		this.resizeObserver.disconnect();
		this.canvas.removeEventListener("pointerdown", this.onPointerDown);
		this.canvas.removeEventListener("pointerup", this.pick);
		this.camera.detach();
		this.fields?.dispose();
		this.particles?.dispose();
		this.boundaryBuffer?.destroy();
		this.depthTexture?.destroy();
		this.gpu?.device.destroy();
		this.canvas.remove();
	}

	private async initialize() {
		try {
			const gpu = await connectFluidGPU(this.canvas);

			if (this.disposed) {
				gpu.device.destroy();
				return;
			}

			this.gpu = gpu;
			this.fields = new FluidFieldView(gpu);
			this.particles = new FluidParticles(gpu);
			this.fields.setOptions(this.options);
			this.particles.visible = this.options.particles;
			this.createBoundary(gpu);

			if (this.queuedFields !== null) {
				this.fields.update(this.queuedFields);
				this.particles.setGridSpacing(this.queuedFields.grid.spacing);
			}

			if (this.queuedParticles !== null) {
				this.particles.update(this.queuedParticles);
			}

			this.resize();
			this.invalidate();
		} catch (cause) {
			const error = cause instanceof Error ? cause : new Error(String(cause));
			this.onError?.(error);
		}
	}

	private createBoundary(gpu: FluidGPU) {
		if (this.fields === null) {
			return;
		}

		this.boundaryBuffer = createVertexBuffer(gpu.device, boundaryLines());
		const layout = gpu.device.createBindGroupLayout({
			entries: [
				{
					binding: 0,
					visibility: GPUShaderStage.VERTEX,
					buffer: { type: "uniform" },
				},
			],
		});
		this.boundaryBindGroup = gpu.device.createBindGroup({
			layout,
			entries: [{ binding: 0, resource: { buffer: this.fields.uniformBuffer } }],
		});
		this.boundaryPipeline = gpu.device.createRenderPipeline({
			layout: gpu.device.createPipelineLayout({ bindGroupLayouts: [layout] }),
			vertex: {
				module: gpu.device.createShaderModule({ code: lineShader }),
				entryPoint: "vs_main",
				buffers: [
					{
						arrayStride: 12,
						attributes: [{ shaderLocation: 0, offset: 0, format: "float32x3" }],
					},
				],
			},
			fragment: {
				module: gpu.device.createShaderModule({ code: lineShader }),
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
			primitive: { topology: "line-list" },
			depthStencil: {
				format: "depth24plus",
				depthWriteEnabled: false,
				depthCompare: "less",
			},
		});
	}

	private readonly resize = () => {
		const width = this.container.clientWidth;
		const height = this.container.clientHeight;

		if (width === 0 || height === 0) {
			return;
		}

		const pixelRatio = window.devicePixelRatio;
		this.canvas.width = Math.max(1, Math.floor(width * pixelRatio));
		this.canvas.height = Math.max(1, Math.floor(height * pixelRatio));
		this.camera.setAspect(width / height);
		this.rebuildDepth();
		this.invalidate();
	};

	private rebuildDepth() {
		const gpu = this.gpu;

		if (gpu === null) {
			return;
		}

		const width = this.canvas.width;
		const height = this.canvas.height;

		if (
			this.depthTexture !== null &&
			this.depthTexture.width === width &&
			this.depthTexture.height === height
		) {
			return;
		}

		this.depthTexture?.destroy();
		this.depthTexture = gpu.device.createTexture({
			size: { width, height },
			format: "depth24plus",
			usage: GPUTextureUsage.RENDER_ATTACHMENT,
		});
	}

	private readonly render = () => {
		this.animationFrame = 0;
		const gpu = this.gpu;
		const fields = this.fields;
		const particles = this.particles;
		const depth = this.depthTexture;

		if (gpu === null || fields === null || particles === null || depth === null) {
			return;
		}

		fields.writeFrame(this.camera.viewProj, this.camera.invViewProj, this.camera.position);
		particles.writeFrame(this.camera);
		const encoder = gpu.device.createCommandEncoder();
		const pass = encoder.beginRenderPass({
			colorAttachments: [
				{
					view: gpu.context.getCurrentTexture().createView(),
					clearValue: CLEAR_COLOR,
					loadOp: "clear",
					storeOp: "store",
				},
			],
			depthStencilAttachment: {
				view: depth.createView(),
				depthClearValue: 1,
				depthLoadOp: "clear",
				depthStoreOp: "store",
			},
		});
		fields.encode(pass);
		particles.encode(pass);

		if (this.boundaryPipeline !== null && this.boundaryBindGroup !== null && this.boundaryBuffer !== null) {
			pass.setPipeline(this.boundaryPipeline);
			pass.setBindGroup(0, this.boundaryBindGroup);
			pass.setVertexBuffer(0, this.boundaryBuffer);
			pass.draw(24);
		}

		pass.end();
		gpu.device.queue.submit([encoder.finish()]);
	};

	private readonly invalidate = () => {
		if (this.animationFrame !== 0 || this.disposed) {
			return;
		}

		this.animationFrame = requestAnimationFrame(this.render);
	};

	private readonly onPointerDown = (event: PointerEvent) => {
		this.pointerDownX = event.clientX;
		this.pointerDownY = event.clientY;
	};

	private readonly pick = (event: PointerEvent) => {
		if (this.particles === null) {
			return;
		}

		if (Math.hypot(event.clientX - this.pointerDownX, event.clientY - this.pointerDownY) > 4) {
			return;
		}

		const bounds = this.canvas.getBoundingClientRect();
		const ndcX = ((event.clientX - bounds.left) / bounds.width) * 2 - 1;
		const ndcY = -(((event.clientY - bounds.top) / bounds.height) * 2 - 1);
		this.onSelect(this.particles.pick(this.camera.ray(ndcX, ndcY)));
	};
}
