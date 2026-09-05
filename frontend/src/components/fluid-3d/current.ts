import { currentShader } from "./field-shaders";
import { createVertexBuffer, type FluidGPU } from "./gpu";
import type { FluidFields } from "./wire";

/*
PhaseCurrent visualizes the pilot-wave guidance current — the phase gradient
field j = ψRe·∇ψIm − ψIm·∇ψRe — as a stream of short bright streaks that are
advected along it each frame. The field is derived once per fields update from
the real and imaginary wave arrays the kernel already publishes, so the stream
is a faithful picture of the actual current, not a decorative animation. The
streaks ride where |ψ|² is present; in empty regions a marker reseeds so the
stream never degenerates into frozen noise.
*/
export class PhaseCurrent {
	visible = true;

	private readonly markerCount = 1600;
	private readonly maxSpeed = 0.012;
	private readonly streakLength = 0.014;
	private readonly jFloor = 1e-7;

	private markers: Float32Array;
	private vertices: Float32Array;
	private currentX: Float32Array | null = null;
	private currentY: Float32Array | null = null;
	private currentZ: Float32Array | null = null;
	private grid: { x: number; y: number; z: number; spacing: number } | null =
		null;
	private jPeak = 0;
	private vertexBuffer: GPUBuffer | null = null;
	private pipeline: GPURenderPipeline | null = null;
	private bindGroup: GPUBindGroup | null = null;

	constructor(
		private readonly gpu: FluidGPU,
		fieldsUniformBuffer: GPUBuffer,
	) {
		this.markers = new Float32Array(this.markerCount * 3);
		this.vertices = new Float32Array(this.markerCount * 2 * 5);

		for (let index = 0; index < this.markerCount; index++) {
			this.markers[index * 3 + 0] = Math.random();
			this.markers[index * 3 + 1] = Math.random();
			this.markers[index * 3 + 2] = Math.random();
		}

		const layout = this.gpu.device.createBindGroupLayout({
			entries: [
				{
					binding: 0,
					visibility: GPUShaderStage.VERTEX,
					buffer: { type: "uniform" },
				},
			],
		});
		this.bindGroup = this.gpu.device.createBindGroup({
			layout,
			entries: [{ binding: 0, resource: { buffer: fieldsUniformBuffer } }],
		});
		this.pipeline = this.gpu.device.createRenderPipeline({
			layout: this.gpu.device.createPipelineLayout({
				bindGroupLayouts: [layout],
			}),
			vertex: {
				module: this.gpu.device.createShaderModule({
					code: currentShader,
				}),
				entryPoint: "vs_main",
				buffers: [
					{
						arrayStride: 20,
						attributes: [
							{ shaderLocation: 0, offset: 0, format: "float32x3" },
							{ shaderLocation: 1, offset: 12, format: "float32x2" },
						],
					},
				],
			},
			fragment: {
				module: this.gpu.device.createShaderModule({
					code: currentShader,
				}),
				entryPoint: "fs_main",
				targets: [
					{
						format: this.gpu.format,
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
			primitive: { topology: "line-list" },
			depthStencil: {
				format: "depth24plus",
				depthWriteEnabled: false,
				depthCompare: "less",
			},
		});
	}

	/*
	update recomputes j(x) = ψRe·∇ψIm − ψIm·∇ψRe on the current wave field with
	periodic central differences.
	*/
	update(fields: FluidFields) {
		const { x: gx, y: gy, z: gz, spacing } = fields.grid;
		const cells = gx * gy * gz;
		const waveReal = fields.waveReal;
		const waveImaginary = fields.waveImaginary;

		if (
			this.currentX === null ||
			this.currentY === null ||
			this.currentZ === null ||
			this.currentX.length !== cells
		) {
			this.currentX = new Float32Array(cells);
			this.currentY = new Float32Array(cells);
			this.currentZ = new Float32Array(cells);
		}

		const currentX = this.currentX;
		const currentY = this.currentY;
		const currentZ = this.currentZ;
		const halfSpacing = 0.5 / spacing;

		let peak = 0;

		for (let z = 0; z < gz; z++) {
			const zMinus = (z - 1 + gz) % gz;
			const zPlus = (z + 1) % gz;

			for (let y = 0; y < gy; y++) {
				const yMinus = (y - 1 + gy) % gy;
				const yPlus = (y + 1) % gy;

				for (let x = 0; x < gx; x++) {
					const xMinus = (x - 1 + gx) % gx;
					const xPlus = (x + 1) % gx;
					const cell = x + gx * (y + gy * z);

					const realCenter = waveReal[cell];
					const imagCenter = waveImaginary[cell];

					const dRealX =
						(waveReal[xPlus + gx * (y + gy * z)] -
							waveReal[xMinus + gx * (y + gy * z)]) *
						halfSpacing;
					const dImagX =
						(waveImaginary[xPlus + gx * (y + gy * z)] -
							waveImaginary[xMinus + gx * (y + gy * z)]) *
						halfSpacing;
					const dRealY =
						(waveReal[x + gx * (yPlus + gy * z)] -
							waveReal[x + gx * (yMinus + gy * z)]) *
						halfSpacing;
					const dImagY =
						(waveImaginary[x + gx * (yPlus + gy * z)] -
							waveImaginary[x + gx * (yMinus + gy * z)]) *
						halfSpacing;
					const dRealZ =
						(waveReal[x + gx * (y + gy * zPlus)] -
							waveReal[x + gx * (y + gy * zMinus)]) *
						halfSpacing;
					const dImagZ =
						(waveImaginary[x + gx * (y + gy * zPlus)] -
							waveImaginary[x + gx * (y + gy * zMinus)]) *
						halfSpacing;

					const jx = realCenter * dImagX - imagCenter * dRealX;
					const jy = realCenter * dImagY - imagCenter * dRealY;
					const jz = realCenter * dImagZ - imagCenter * dRealZ;

					currentX[cell] = jx;
					currentY[cell] = jy;
					currentZ[cell] = jz;

					const magnitude = Math.hypot(jx, jy, jz);

					if (magnitude > peak) {
						peak = magnitude;
					}
				}
			}
		}

		this.grid = { x: gx, y: gy, z: gz, spacing };
		this.jPeak = peak;
	}

	/*
	stepAndEncode advects the markers along the current field, builds the streak
	vertex buffer, and draws it additively.
	*/
	stepAndEncode(pass: GPURenderPassEncoder) {
		if (
			!this.visible ||
			this.currentX === null ||
			this.currentY === null ||
			this.currentZ === null ||
			this.grid === null ||
			this.pipeline === null ||
			this.bindGroup === null ||
			this.jPeak <= 0
		) {
			return;
		}

		const gx = this.grid.x;
		const gy = this.grid.y;
		const gz = this.grid.z;
		const currentX = this.currentX;
		const currentY = this.currentY;
		const currentZ = this.currentZ;
		const markers = this.markers;
		const vertices = this.vertices;
		const speedScale = this.maxSpeed / this.jPeak;
		const lengthScale = this.streakLength / this.jPeak;

		for (let index = 0; index < this.markerCount; index++) {
			const px = markers[index * 3 + 0];
			const py = markers[index * 3 + 1];
			const pz = markers[index * 3 + 2];
			const ix = Math.min(gx - 1, Math.max(0, Math.floor(px * gx)));
			const iy = Math.min(gy - 1, Math.max(0, Math.floor(py * gy)));
			const iz = Math.min(gz - 1, Math.max(0, Math.floor(pz * gz)));
			const cell = ix + gx * (iy + gy * iz);
			const jx = currentX[cell];
			const jy = currentY[cell];
			const jz = currentZ[cell];
			const magnitude = Math.hypot(jx, jy, jz);

			if (magnitude <= this.jFloor) {
				markers[index * 3 + 0] = Math.random();
				markers[index * 3 + 1] = Math.random();
				markers[index * 3 + 2] = Math.random();

				// Zero-length streak at the new seed so no stale segment
				// lingers where the current has died.
				const reseedVertex = index * 10;
				vertices[reseedVertex + 0] = markers[index * 3 + 0];
				vertices[reseedVertex + 1] = markers[index * 3 + 1];
				vertices[reseedVertex + 2] = markers[index * 3 + 2];
				vertices[reseedVertex + 3] = 0;
				vertices[reseedVertex + 4] = 0;
				vertices[reseedVertex + 5] = markers[index * 3 + 0];
				vertices[reseedVertex + 6] = markers[index * 3 + 1];
				vertices[reseedVertex + 7] = markers[index * 3 + 2];
				vertices[reseedVertex + 8] = 0;
				vertices[reseedVertex + 9] = 0;
				continue;
			}

			const stepX = jx * speedScale;
			const stepY = jy * speedScale;
			const stepZ = jz * speedScale;
			const nextX = px + stepX;
			const nextY = py + stepY;
			const nextZ = pz + stepZ;

			markers[index * 3 + 0] = nextX - Math.floor(nextX);
			markers[index * 3 + 1] = nextY - Math.floor(nextY);
			markers[index * 3 + 2] = nextZ - Math.floor(nextZ);

			const directionX = jx / magnitude;
			const directionY = jy / magnitude;
			const directionZ = jz / magnitude;
			const tailX = px - directionX * magnitude * lengthScale;
			const tailY = py - directionY * magnitude * lengthScale;
			const tailZ = pz - directionZ * magnitude * lengthScale;
			const normSpeed = magnitude / (this.jPeak + 1e-6);
			const vertex = index * 10;

			// Head vertex: position, tail = 1.0, normalized speed.
			vertices[vertex + 0] = px;
			vertices[vertex + 1] = py;
			vertices[vertex + 2] = pz;
			vertices[vertex + 3] = 1;
			vertices[vertex + 4] = normSpeed;

			// Tail vertex: position, tail = 0.0, normalized speed.
			vertices[vertex + 5] = tailX;
			vertices[vertex + 6] = tailY;
			vertices[vertex + 7] = tailZ;
			vertices[vertex + 8] = 0;
			vertices[vertex + 9] = normSpeed;
		}

		if (this.vertexBuffer === null) {
			this.vertexBuffer = createVertexBuffer(this.gpu.device, this.vertices);
		} else {
			this.gpu.device.queue.writeBuffer(this.vertexBuffer, 0, this.vertices);
		}

		pass.setPipeline(this.pipeline);
		pass.setBindGroup(0, this.bindGroup);
		pass.setVertexBuffer(0, this.vertexBuffer);
		pass.draw(this.markerCount * 2);
	}

	dispose() {
		this.vertexBuffer?.destroy();
	}
}
