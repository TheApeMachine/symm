// @vitest-environment jsdom
import {
	act,
	cleanup,
	fireEvent,
	render,
	screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	gateLabStore,
	RESONANCE_DEFAULTS,
	RISK_DEFAULTS,
	resetGateLab,
} from "#/collections/gate-lab";
import { GateLab, GateLabToggle } from "./gate-lab";

const send = vi.fn();

vi.mock("#/providers/websocket", () => ({
	trySendCommand: (command: Record<string, unknown>) => {
		send(command);
		return true;
	},
}));

afterEach(() => {
	cleanup();
	act(() => {
		resetGateLab();
		gateLabStore.setState((prev) => ({ ...prev, open: false }));
	});
	send.mockClear();
});

describe("GateLab", () => {
	it("toggles open and cycles an oracle override", () => {
		render(
			<>
				<GateLabToggle />
				<GateLab />
			</>,
		);

		expect(screen.queryByTestId("gate-lab")).toBeNull();
		fireEvent.click(screen.getByText("Lab"));
		expect(document.querySelector('[data-gate-lab="true"]')).toBeTruthy();

		const vol = document.querySelector(
			'[data-gate-lab-oracle="vol"]',
		) as HTMLButtonElement;
		expect(vol.getAttribute("data-gate-lab-override")).toBe("auto");
		fireEvent.click(vol);
		expect(vol.getAttribute("data-gate-lab-override")).toBe("veto");
		fireEvent.click(vol);
		expect(vol.getAttribute("data-gate-lab-override")).toBe("approve");
	});

	it("applies risk draft to the hub", () => {
		act(() => {
			gateLabStore.setState((prev) => ({ ...prev, open: true }));
		});

		render(<GateLab />);

		fireEvent.click(screen.getByText("Apply risk"));

		expect(send).toHaveBeenCalledWith({
			type: "risk",
			...RISK_DEFAULTS,
		});
	});

	it("applies resonance draft to the hub without bypass", () => {
		act(() => {
			gateLabStore.setState((prev) => ({ ...prev, open: true }));
		});

		render(<GateLab />);

		expect(document.querySelector("[data-gate-lab-rez-bypass]")).toBeNull();

		fireEvent.click(screen.getByText("Apply rez"));

		expect(send).toHaveBeenCalledWith({
			type: "resonance",
			...RESONANCE_DEFAULTS,
		});
	});
});
