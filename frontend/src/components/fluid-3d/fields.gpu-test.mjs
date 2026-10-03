import { FluidFieldView } from "./fields.ts";
import { connectFluidGPU } from "./gpu.ts";

// Run against the real WebGPU renderer from the localhost browser console:
// await (await import('/src/components/fluid-3d/fields.gpu-test.mjs')).verifyFieldRendering()
// No scene state or visible canvas is modified.
export const verifyFieldRendering = async () => {
	const gpu = await connectFluidGPU(document.createElement("canvas"));
	const device = gpu.device;
	const results = [];
	try {
		const cases = [
			{ axis: 1, amplitude: 0 },
			{ axis: 1, amplitude: 1 },
			{ axis: 4, amplitude: 1 },
			{ axis: 4, amplitude: 1, exposure: 4, phase: Math.PI / 2 },
			{ axis: 1, gas: true, density: 0, momentum: 100 },
			{ axis: 1, gas: true, density: 1 },
			{ axis: 4, gas: true, density: 1 },
			{ axis: 64, gas: true, density: 1606340, slab: true },
			{ axis: 4, gas: true, density: 1e-8, exposure: 4 },
			{
				axis: 4,
				amplitude: 1,
				slice: 0,
				expectedPhase: 0,
				expectedAmplitude: 1,
			},
			{
				axis: 4,
				amplitude: 1,
				slice: 0.125,
				expectedPhase: Math.PI / 4,
				expectedAmplitude: Math.SQRT1_2,
			},
			{
				axis: 4,
				amplitude: 1,
				slice: 0.875,
				expectedPhase: -Math.PI / 4,
				expectedAmplitude: Math.SQRT1_2,
			},
		];
		for (const {
			axis,
			amplitude = 0,
			exposure = 1,
			phase = 0,
			gas = false,
			density = 0,
			momentum = 0,
			slab = false,
			slice,
			expectedPhase = phase,
			expectedAmplitude = amplitude,
		} of cases) {
			const count = axis ** 3;
			const field = new FluidFieldView({ ...gpu, format: "rgba8unorm" });
			const target = device.createTexture({
				size: [1, 1],
				format: "rgba8unorm",
				usage: GPUTextureUsage.RENDER_ATTACHMENT | GPUTextureUsage.COPY_SRC,
			});
			const depth = device.createTexture({
				size: [1, 1],
				format: "depth24plus",
				usage: GPUTextureUsage.RENDER_ATTACHMENT,
			});
			const readback = device.createBuffer({
				size: 256,
				usage: GPUBufferUsage.COPY_DST | GPUBufferUsage.MAP_READ,
			});
			device.pushErrorScope("validation");
			try {
				const momRho = new Float32Array(4 * count);
				for (let cell = 0; cell < count; cell++) {
					momRho[cell * 4] = momentum;
					momRho[cell * 4 + 3] =
						!slab || cell % axis === Math.floor(axis / 2) ? density : 0;
				}
				field.update({
					grid: { x: axis, y: axis, z: axis, spacing: 1 / axis },
					momRho,
					internalEnergy: new Float32Array(count).fill(density),
					waveReal: Float32Array.from(
						{ length: count },
						(_, cell) =>
							amplitude *
							Math.cos(
								slice === undefined
									? phase
									: (2 * Math.PI * (cell % axis)) / axis,
							),
					),
					waveImaginary: Float32Array.from(
						{ length: count },
						(_, cell) =>
							amplitude *
							Math.sin(
								slice === undefined
									? phase
									: (2 * Math.PI * (cell % axis)) / axis,
							),
					),
					densityScale: density,
					momentumScale: momentum,
					energyScale: density,
					waveScale: 1,
				});
				field.setOptions({
					gas,
					wave: !gas,
					volume: slice === undefined,
					slices: slice !== undefined,
					exposure,
				});
				if (slice !== undefined) field.setSlices(0.5, 0.5, slice);
				const projection = new Float32Array([
					2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0.5, 0, -1, -1, 0, 1,
				]);
				field.writeFrame(projection, projection, [0.5, 0.5, 2]);
				const encoder = device.createCommandEncoder();
				const pass = encoder.beginRenderPass({
					colorAttachments: [
						{
							view: target.createView(),
							clearValue: { r: 0, g: 0, b: 0, a: 0 },
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
				field.encode(pass);
				pass.end();
				encoder.copyTextureToBuffer(
					{ texture: target },
					{ buffer: readback, bytesPerRow: 256 },
					[1, 1],
				);
				device.queue.submit([encoder.finish()]);
				await readback.mapAsync(GPUMapMode.READ);
				const actual = Array.from(
					new Uint8Array(readback.getMappedRange()).slice(0, 4),
				);
				readback.unmap();
				const opticalDepth = gas ? Number(density > 0) : expectedAmplitude;
				const alpha = 1 - Math.exp(-opticalDepth * exposure);
				const tint = gas
					? [0.95, 0.55, 0.12]
					: [0, (2 * Math.PI) / 3, (4 * Math.PI) / 3].map(
							(offset) => (0.5 + 0.5 * Math.cos(expectedPhase - offset)) ** 1.2,
						);
				const expected = tint.map((value) => Math.round(255 * alpha * value));
				expected.push(Math.round(255 * alpha));
				// One byte is the output format's quantization unit.
				if (
					actual.some((value, index) => Math.abs(value - expected[index]) > 1)
				) {
					throw new Error(
						`Volume integration mismatch: grid=${axis}, actual=${actual}, expected=${expected}`,
					);
				}
				results.push({
					axis,
					slice,
					gas,
					density,
					slab,
					amplitude,
					exposure,
					actual,
					expected,
				});
			} finally {
				field.dispose();
				target.destroy();
				depth.destroy();
				readback.destroy();
				const error = await device.popErrorScope();
				if (error) throw new Error(error.message);
			}
		}
		return { passed: results.length, cases: results };
	} finally {
		device.destroy();
	}
};
