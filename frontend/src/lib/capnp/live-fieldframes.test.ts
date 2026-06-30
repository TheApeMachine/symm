import { describe, expect, it } from "vitest";
import { decodePackedArtifactWire } from "./read-artifact";

const describeLive = process.env.SYMM_LIVE_WS_TEST === "1" ? describe : describe.skip;
const liveWebsocketUrl =
	process.env.SYMM_LIVE_WS_URL ?? "ws://127.0.0.1:8765/ws";

describeLive("Live field frame invariants", () => {
	it("publishes backend manifold and resonance frames", async () => {
		const ws = new WebSocket(liveWebsocketUrl);
		ws.binaryType = "arraybuffer";

		const observed = await new Promise<{
			manifold: boolean;
			resonance: boolean;
		}>((resolve, reject) => {
			const state = {
				manifold: false,
				resonance: false,
			};
			const timeout = setTimeout(() => {
				ws.close();
				reject(
					new Error(
						`timed out waiting for field frames: ${JSON.stringify(state)}`,
					),
				);
			}, 90000);
			const finishIfReady = () => {
				if (!state.manifold || !state.resonance) {
					return;
				}

				clearTimeout(timeout);
				ws.close();
				resolve(state);
			};

			ws.onmessage = async (event) => {
				try {
					const frame = await decodePackedArtifactWire(event.data as ArrayBuffer);

					if (frame === null) {
						return;
					}

					if (frame.role === "manifold") {
						expect(frame.type).toBe("manifold");
						expect(Array.isArray(frame.rho)).toBe(true);
						expect(Array.isArray(frame.carriers)).toBe(true);
						state.manifold = true;
					}

					if (frame.role === "resonance") {
						expect(frame.type).toBe("resonance_universe");
						expect(Array.isArray(frame.snapshots)).toBe(true);
						state.resonance = true;
					}

					finishIfReady();
				} catch (error) {
					clearTimeout(timeout);
					ws.close();
					reject(error);
				}
			};

			ws.onerror = (error) => {
				clearTimeout(timeout);
				reject(error);
			};
		});

		expect(observed.manifold).toBe(true);
		expect(observed.resonance).toBe(true);
	}, 95000);
});
