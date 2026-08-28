import type { OrbitCamera, Ray } from "./camera";
import { particleShader } from "./field-shaders";
import {
	createUniformBuffer,
	createVertexBuffer,
	type FluidGPU,
	PARTICLE_UNIFORM_FLOATS,
} from "./gpu";
import type { FluidParticleFrame } from "./wire";

const quadCorners = new Float32Array([
	-0.5, -0.5, 0.5, -0.5, -0.5, 0.5, 0.5, 0.5,
]);
const PARTICLE_STRIDE_BYTES = 48;

const particleBindLayout = (device: GPUDevice) =>
	device.createBindGroupLayout({
		entries: [
			{
				binding: 0,
				visibility: GPUShaderStage.VERTEX,
				buffer: { type: "uniform" },
			},
		],
	});

/*
FluidParticles is one instanced-quad draw for the complete resident particle
selection. WebGPU has no point size, so each particle is a camera-facing quad.
*/
const maxAbs = (values: Float32Array): number => {
	let peak = 0;

	for (const value of values) {
		const abs = Math.abs(value);

		if (abs > peak) {
			peak = abs;
		}
	}

	return peak;
};

export class FluidParticles {
	visible = true;
	private frame: FluidParticleFrame | null = null;
	private instanceBuffer: GPUBuffer | null = null;
	private instanceCapacity = 0;
	private gridSpacing = 1 / 64;
	private scales = { heat: 0, energy: 0, mass: 0, amplitude: 0 };
	private readonly quadBuffer: GPUBuffer;
	private readonly uniformBuffer: GPUBuffer;
	private readonly bindGroup: GPUBindGroup;
	private readonly pipeline: GPURenderPipeline;

	constructor(private readonly gpu: FluidGPU) {
		const layout = particleBindLayout(gpu.device);
		this.quadBuffer = createVertexBuffer(gpu.device, quadCorners);
		this.uniformBuffer = createUniformBuffer(
			gpu.device,
			PARTICLE_UNIFORM_FLOATS,
		);
		this.bindGroup = gpu.device.createBindGroup({
			layout,
			entries: [{ binding: 0, resource: { buffer: this.uniformBuffer } }],
		});
		this.pipeline = gpu.device.createRenderPipeline({
			layout: gpu.device.createPipelineLayout({ bindGroupLayouts: [layout] }),
			vertex: {
				module: gpu.device.createShaderModule({ code: particleShader }),
				entryPoint: "vs_main",
				buffers: [
					{
						arrayStride: 8,
						stepMode: "vertex",
						attributes: [{ shaderLocation: 0, offset: 0, format: "float32x2" }],
					},
					{
						arrayStride: PARTICLE_STRIDE_BYTES,
						stepMode: "instance",
						attributes: [
							{ shaderLocation: 1, offset: 0, format: "float32x3" },
							{ shaderLocation: 2, offset: 24, format: "float32" },
							{ shaderLocation: 3, offset: 28, format: "float32" },
							{ shaderLocation: 4, offset: 32, format: "float32" },
							{ shaderLocation: 5, offset: 36, format: "float32" },
							{ shaderLocation: 6, offset: 44, format: "float32" },
						],
					},
				],
			},
			fragment: {
				module: gpu.device.createShaderModule({ code: particleShader }),
				entryPoint: "fs_main",
				targets: [
					{
						format: gpu.format,
						blend: {
							color: {
								srcFactor: "src-alpha",
								dstFactor: "one",
								operation: "add",
							},
							alpha: {
								srcFactor: "one",
								dstFactor: "one",
								operation: "add",
							},
						},
					},
				],
			},
			primitive: { topology: "triangle-strip" },
			depthStencil: {
				format: "depth24plus",
				depthWriteEnabled: false,
				depthCompare: "less",
			},
		});
	}

	update(frame: FluidParticleFrame) {
		if (frame.count === 0) {
			this.frame = frame;
			return;
		}

		const bytes = frame.count * PARTICLE_STRIDE_BYTES;

		if (this.instanceBuffer === null || this.instanceCapacity < bytes) {
			this.instanceBuffer?.destroy();
			this.instanceCapacity = bytes;
			this.instanceBuffer = this.gpu.device.createBuffer({
				size: bytes,
				usage: GPUBufferUsage.VERTEX | GPUBufferUsage.COPY_DST,
			});
		}

		// The wire carries one Float32Array per field; the vertex buffer needs
		// them interleaved (Pos3, Vel3, Mass, Heat, Energy, Phase, Omega, Amp)
		// for GPU instancing. Interleaving is GPU vertex-buffer layout, done
		// here at the one place that needs those bytes — the wire format
		// itself stays field-for-field, unreshaped.
		const stride = PARTICLE_STRIDE_BYTES / Float32Array.BYTES_PER_ELEMENT;
		const instances = new Float32Array(frame.count * stride);

		for (let index = 0; index < frame.count; index += 1) {
			const p = index * 3;
			const o = index * stride;
			instances[o + 0] = frame.pos[p + 0] ?? 0;
			instances[o + 1] = frame.pos[p + 1] ?? 0;
			instances[o + 2] = frame.pos[p + 2] ?? 0;
			instances[o + 3] = frame.vel[p + 0] ?? 0;
			instances[o + 4] = frame.vel[p + 1] ?? 0;
			instances[o + 5] = frame.vel[p + 2] ?? 0;
			instances[o + 6] = frame.mass[index] ?? 0;
			instances[o + 7] = frame.heat[index] ?? 0;
			instances[o + 8] = frame.energy[index] ?? 0;
			instances[o + 9] = frame.phase[index] ?? 0;
			instances[o + 10] = frame.omega[index] ?? 0;
			instances[o + 11] = frame.amp[index] ?? 0;
		}

		this.gpu.device.queue.writeBuffer(this.instanceBuffer, 0, instances);
		this.frame = frame;
		this.scales = {
			heat: maxAbs(frame.heat),
			energy: maxAbs(frame.energy),
			mass: maxAbs(frame.mass),
			amplitude: maxAbs(frame.amp),
		};
	}

	setGridSpacing(spacing: number) {
		this.gridSpacing = spacing;
	}

	writeFrame(camera: OrbitCamera) {
		const data = new Float32Array(PARTICLE_UNIFORM_FLOATS);
		data.set(camera.viewProj, 0);
		data[16] = camera.right[0];
		data[17] = camera.right[1];
		data[18] = camera.right[2];
		data[19] = this.gridSpacing;
		data[20] = camera.up[0];
		data[21] = camera.up[1];
		data[22] = camera.up[2];
		data[23] = this.scales.heat;
		data[24] = this.scales.energy;
		data[25] = this.scales.mass;
		data[26] = this.scales.amplitude;
		this.gpu.device.queue.writeBuffer(this.uniformBuffer, 0, data);
	}

	encode(pass: GPURenderPassEncoder) {
		if (
			!this.visible ||
			this.frame === null ||
			this.frame.count === 0 ||
			this.instanceBuffer === null
		) {
			return;
		}

		pass.setPipeline(this.pipeline);
		pass.setBindGroup(0, this.bindGroup);
		pass.setVertexBuffer(0, this.quadBuffer);
		pass.setVertexBuffer(1, this.instanceBuffer);
		pass.draw(4, this.frame.count);
	}

	pick(ray: Ray) {
		if (!this.visible || this.frame === null) {
			return null;
		}

		const threshold = this.gridSpacing;
		let bestIndex = -1;
		let bestAlong = Number.POSITIVE_INFINITY;

		for (let index = 0; index < this.frame.count; index += 1) {
			const particle = this.frame.particle(index);

			if (particle === null) {
				continue;
			}

			const toX = particle.Position.X - ray.origin[0];
			const toY = particle.Position.Y - ray.origin[1];
			const toZ = particle.Position.Z - ray.origin[2];
			const along =
				toX * ray.direction[0] +
				toY * ray.direction[1] +
				toZ * ray.direction[2];

			if (along < 0) {
				continue;
			}

			const closestX = ray.origin[0] + ray.direction[0] * along;
			const closestY = ray.origin[1] + ray.direction[1] * along;
			const closestZ = ray.origin[2] + ray.direction[2] * along;
			const distance = Math.hypot(
				particle.Position.X - closestX,
				particle.Position.Y - closestY,
				particle.Position.Z - closestZ,
			);

			if (distance > threshold || along >= bestAlong) {
				continue;
			}

			bestAlong = along;
			bestIndex = index;
		}

		return bestIndex < 0 ? null : this.frame.particle(bestIndex);
	}

	dispose() {
		this.instanceBuffer?.destroy();
		this.quadBuffer.destroy();
		this.uniformBuffer.destroy();
	}
}
