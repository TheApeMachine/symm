import { describe, expect, it } from "vitest";
import { decodePackedArtifactWire } from "./read-artifact";

const describeLive = process.env.SYMM_LIVE_WS_TEST === "1" ? describe : describe.skip;
const liveWebsocketUrl =
	process.env.SYMM_LIVE_WS_URL ?? "ws://127.0.0.1:8765/ws";

describeLive("Live WebSocket debug", () => {
	it("connects and decodes raw frames", async () => {
		const ws = new WebSocket(liveWebsocketUrl);
		ws.binaryType = "arraybuffer";

		const messagePromise = new Promise<void>((resolve, reject) => {
			const timeout = setTimeout(() => {
				ws.close();
				reject(new Error("timed out waiting for tick frame"));
			}, 10000);

			const finish = () => {
				clearTimeout(timeout);
				ws.close();
				resolve();
			};

			ws.onmessage = async (event) => {
				try {
					const buffer = event.data as ArrayBuffer;
					console.log(`Received message of length ${buffer.byteLength}`);
					const frame = await decodePackedArtifactWire(buffer);
					console.log("Decoded frame:", JSON.stringify(frame, null, 2));
					expect(frame).not.toBeNull();
					expect(typeof frame?.role).toBe("string");
					if (frame?.role === "tick") {
						expect(typeof frame.count).toBe("number");
						finish();
					}
				} catch (err) {
					console.error("Decode failed:", err);
					ws.close();
					reject(err);
				}
			};

			ws.onerror = (err) => {
				console.error("WS error:", err);
				reject(err);
			};

			ws.onclose = () => {
				console.log("WS closed");
			};
		});

		await messagePromise;
	}, 12000);
});
