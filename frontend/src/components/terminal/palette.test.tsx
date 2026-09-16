// @vitest-environment jsdom
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { terminalStore } from "#/collections/terminal";
import { CommandPalette } from "./palette";

afterEach(() => {
	cleanup();
	terminalStore.actions.closePalette();
});

describe("CommandPalette", () => {
	it("offers active surfaces without linking to retired graph or regulator pages", () => {
		terminalStore.actions.openPalette();
		const onRun = vi.fn();
		render(<CommandPalette activeSurface="dashboard" onRun={onRun} />);
		expect(screen.queryByText("Market graph")).toBeNull();
		expect(screen.queryByText("Global regulator")).toBeNull();
		fireEvent.click(screen.getByText("Latent x-ray"));
		expect(onRun).toHaveBeenCalledWith("xray", undefined, undefined);
	});
});
