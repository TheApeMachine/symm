// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { errorAtom } from "#/collections/app";
import { Dialog } from "./dialog";

describe("Dialog (Backend error bar)", () => {
	beforeEach(() => {
		errorAtom.set(null);
	});

	afterEach(() => {
		cleanup();
	});

	it("renders nothing when there is no error", () => {
		const { container } = render(<Dialog />);
		expect(container.firstChild).toBeNull();
	});

	it("renders a compact bar under the top bar when an error is present", () => {
		errorAtom.set({
			source: "WebSocket Shard 1",
			url: "ws://localhost:8765/ws/1",
			error: "Connection refused",
		});

		render(<Dialog />);

		expect(screen.getByText("BACKEND ERROR")).toBeTruthy();
		expect(
			screen.getByText("WebSocket Shard 1 · ws://localhost:8765/ws/1"),
		).toBeTruthy();
		expect(screen.getByText("details ▼")).toBeTruthy();
		expect(screen.getByText("dismiss")).toBeTruthy();

		// Drawer should initially be folded in
		expect(screen.queryByText("Connection refused")).toBeNull();
	});

	it("folds out error details when clicking details, and collapses on toggle", () => {
		errorAtom.set({
			source: "WebSocket Shard 1",
			url: "ws://localhost:8765/ws/1",
			error: "Connection refused",
		});

		render(<Dialog />);

		const toggleBtn = screen.getByText("details ▼");
		fireEvent.click(toggleBtn);

		// Drawer should now be visible
		expect(screen.getByText("collapse ▲")).toBeTruthy();
		expect(screen.getByText("Connection refused")).toBeTruthy();
		expect(screen.getByText("source:")).toBeTruthy();
		expect(screen.getByText("url:")).toBeTruthy();

		// Clicking collapse closes the drawer
		fireEvent.click(screen.getByText("collapse ▲"));
		expect(screen.getByText("details ▼")).toBeTruthy();
		expect(screen.queryByText("source:")).toBeNull();
	});

	it("clears the error when dismiss is clicked", () => {
		errorAtom.set({
			source: "WebSocket Shard 1",
			url: "ws://localhost:8765/ws/1",
		});

		const { container } = render(<Dialog />);
		expect(screen.getByText("BACKEND ERROR")).toBeTruthy();

		fireEvent.click(screen.getByText("dismiss"));
		expect(errorAtom.get()).toBeNull();
		expect(container.firstChild).toBeNull();
	});
});
