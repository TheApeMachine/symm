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

vi.mock("#/lib/hub-socket", () => ({
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

	it("toggles resonance bypass and auto-applies the draft", () => {
		act(() => {
			gateLabStore.setState((prev) => ({ ...prev, open: true }));
		});

		render(<GateLab />);

		const bypass = document.querySelector(
			"[data-gate-lab-rez-bypass]",
		) as HTMLButtonElement;
		expect(bypass.getAttribute("data-gate-lab-rez-bypass")).toBe("off");
		fireEvent.click(bypass);
		expect(bypass.getAttribute("data-gate-lab-rez-bypass")).toBe("on");

		expect(send).toHaveBeenCalledWith({
			type: "resonance",
			...RESONANCE_DEFAULTS,
			bypass: true,
		});

		fireEvent.click(screen.getByText("Apply rez"));

		expect(send).toHaveBeenCalledWith({
			type: "resonance",
			...RESONANCE_DEFAULTS,
			bypass: true,
		});
	});
});
