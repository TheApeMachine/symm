import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact.capnp";
import { parseGaugeFrame } from "#/collections/signals";

const measurementWireBase64 =
	"EMZQAg//oK4xwqbUuhgAAAAxTSIBAAMRUUoT7QIaEVFiEVVKET0SM/0BsgozpQICATOxAgoCAAH/ZjA3NzI1MzkDLWIzMTQtNDY2MS04NGZhLTBhNDBkODM3D2RkMDT/ZWQ0ODkzMjUDLTk4M2ItNGRmZC05ZTI4LWQyZjNlMTc0DzRhODIDe33/cHVtcGR1bXAAAAD/bWVhc3VyZW0AB2VudP9wdW1wZHVtcAAAAP8y2ZOnppCZCzLt5Lh/Sj/h0fdLxktFMNmKsP9RnoACxpGKyqtLySpfQeAY8GyV/iaLh9lMFQqqhKn7qXlhDWZF9ea5pz7pwV7hAjc+SRQEp70iU6oHLP51nvTVJxOhA5hrTz9vwdsX9LxSoTuQAo9DFcWiqqllDtKOoFX0/11aimbJX6M+XQMraXP5fJ5YzqdjCGS9Isvg2vHTg/Ev4/nO2rbhQE42812VoNHWqkJZ7atfTVvocUAL7x3w9TiDgTULpHqvgifrFkiPr8i+xNQkOTFkyWPPqe8orYfxBMMSUkbqM2yOBZdUAg2hJSXGhcvQijAxDmA0q390JmK4v348giiYE1MZU0iPpkkuWCPdzD6BivFkE+9HvpJtAbDmIRuLjVqZO+eITATd+0wp9MWG9hjQOcS2nSEAwTbfFmt0VxeO6Lc743Byb5BHfOiroWAz/c3qHRsm/xIEf6H+HR+FmRiQKM8LZ0mcisP4RWN8rr16g13fg0zIfRQUTuu8z7dz2s1J+Yg5mwRDGGWWxr1jgE2IXLfCf8eCAc//0PTntM2g9CQnD4/4cvkTZZVCKKLkbHtKuExl5iRAIhfXWYAU53WKOU0CoFLvGN1l8Uzq04ko611aJS5w4bfN0NeiYGQZcNSUINA6OIr6qhyPKZ24JAZaOjrh0XTje65nN66A62A9OHaHXaehUuLqIS5zHeAPgF/ZXb9Gqn7T6LurnjzFWna02nUc0eUSdU6sGwynh0FDqz0wuZekx2C+VFAV9AdkCGzz0zSwQqieOUew2i8H6vC0kPqx0ljGVICG5EMJOTrSSsuXIurzRcTXhYlqSfCvHZ/bNhUJHVKw373g4XDc2D6UIa4eWbWwU4p0f5y+/nSBTWMBoElvmvj6e98QMWLxHp28D1+1gO6gifdxSrL1a3SPXfLndvTqpENEV5ArdVKfpo4u3Ayfh9qGMFmQ1D7zwZSiEZJuXfi9K+weDyRG1kT/htGQdpj1A8ELzaAXz+7zl9GSXYAMjZxh67BGqKI8JK3mBA07OExJiFtnDCO8L23ao1hSQHwQ5IDCE2/OLGq9Srx9XZaQ+gIFJsrbsFQhApIj6aPwyE9NdJuagdjO++fN9AGB/8Stk84JlF6OKYeOQ/xUN9zle31TJj2nOxR8h6P5Vf96jtu/SMrPqoursb+f0zF71pbh1oIO5ZNFKCmoe+GSHWgLrr11H+NqxahLQEhvs4cDtKxoYx6rUrN9ETfGOtWYQg6dgwLKq13JbIUSUXIrM/AMSlAlRZXV7pb2iaRgUmU/vO1ALx5HaUupvtG1dSERAZmvq7Xb4LwhLkus/+j66IcOydr+hmPvrxOZJae4yRyn//Qe9BTtEuGgJqARATlttDeixXa9i88iBGhRC6HLf6LSTSQr7GS3bx3ypqh0JoMbjJRggUzlJ02us+POI7MRPZGN/XVf6zNjgCH0q5jGnCTZMiL4UyaJwKKp0mZ1/srkH9PUb8BkZVupL3j7gQdXK9q6NVxsc7Knbydlkqt1dcmJr1xmKASi9m6FZho8qBB+prtPzx1dXSbpJX8GrqgYW18/IwfSqV1E/98XtJd7aEUyC+ruS4HiwnjoNr0s8tJGMUwv423cb7HPfgTnaXBGhCzSQVF7SpARZyr6v74aBx/aMadRzT2eT32iChN1+KmRP4kf8sFHKYVdx6edVbEa+6FGsldgqKTJBAYB5gN1aQ==";

describe("WsFeed wire decode", () => {
	it("decodes packed hub measurement wire the same way websocket.ts does", async () => {
		const buffer = Uint8Array.from(atob(measurementWireBase64), (char) =>
			char.charCodeAt(0),
		).buffer;

		const message = new capnp.Message(buffer, true);
		const artifact = message.getRoot(Artifact);

		const attributesText = artifact.hasAttributes()
			? new TextDecoder().decode(artifact.getAttributes().toUint8Array()).trim()
			: "";
		const attributesJSON =
			attributesText === "" ? {} : JSON.parse(attributesText);

		let payloadJSON: Record<string, unknown> = {};

		if (artifact.hasEncryptedPayload() && artifact.hasEncryptedKey()) {
			const encryptedKey = new Uint8Array(
				artifact.getEncryptedKey().toUint8Array(),
			);
			const encryptedPayload = new Uint8Array(
				artifact.getEncryptedPayload().toUint8Array(),
			);

			if (encryptedKey.length === 32 && encryptedPayload.length > 12) {
				const cryptoKey = await crypto.subtle.importKey(
					"raw",
					encryptedKey,
					"AES-GCM",
					false,
					["decrypt"],
				);
				const plaintext = await crypto.subtle.decrypt(
					{ name: "AES-GCM", iv: encryptedPayload.slice(0, 12) },
					cryptoKey,
					encryptedPayload.slice(12),
				);
				const payloadText = new TextDecoder().decode(plaintext).trim();

				if (payloadText !== "") {
					payloadJSON = JSON.parse(payloadText) as Record<string, unknown>;
				}
			}
		}

		const frame = {
			...attributesJSON,
			...payloadJSON,
			role: artifact.getRole(),
			scope: artifact.getScope(),
			origin: artifact.getOrigin(),
			destination: artifact.getDestination(),
		};

		expect(frame.origin).toBe("pumpdump");
		expect(frame.role).toBe("measurement");
		expect(frame.samples).toBe(60);
		expect(frame.calibrated).toBe(true);

		const output = frame.output as Record<string, unknown>;

		expect(output.confidence).toBeGreaterThan(0);

		const reading = parseGaugeFrame(frame);

		expect(reading).not.toBeNull();
		expect(reading?.source).toBe("pumpdump");
		expect(reading?.confidence).toBeGreaterThan(0);
	});
});
