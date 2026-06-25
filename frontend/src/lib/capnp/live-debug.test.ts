import { describe, it } from "vitest";
import { decodePackedArtifactWire } from "./read-artifact";

describe("Live WebSocket debug", () => {
	it("connects and decodes raw frames", async () => {
		const ws = new WebSocket("ws://127.0.0.1:8765/ws");
		ws.binaryType = "arraybuffer";

		const messagePromise = new Promise<void>((resolve, reject) => {
			let count = 0;
			ws.onmessage = async (event) => {
				try {
					const buffer = event.data as ArrayBuffer;
					console.log(`Received message of length ${buffer.byteLength}`);
					const frame = await decodePackedArtifactWire(buffer);
					console.log("Decoded frame:", JSON.stringify(frame, null, 2));
					count++;
					if (count >= 5) {
						ws.close();
						resolve();
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
				resolve();
			};

			setTimeout(() => {
				ws.close();
				resolve();
			}, 5000); // limit to 5 seconds
		});

		await messagePromise;
	});
});
