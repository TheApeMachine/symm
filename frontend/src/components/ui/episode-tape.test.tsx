// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EpisodeTape, projectEpisodeTape } from "./episode-tape";
afterEach(cleanup);
describe("EpisodeTape", () => {
	it("renders supplied observations and zero-coordinate annotations, then clears", () => {
		const episode = {
			id: "observed",
			points: [
				{ x: 0, y: 100 },
				{ x: 1, y: 80 },
				{ x: 2, y: 120 },
			],
			markers: [{ id: "A", label: "A", x: 0, y: 100 }],
		};
		const before = JSON.stringify(episode);
		const { container, rerender } = render(<EpisodeTape episode={episode} />);
		expect(container.querySelector("path")?.getAttribute("d")).toContain(
			"M20,130",
		);
		expect(screen.getByText("A")).toBeDefined();
		expect(JSON.stringify(episode)).toBe(before);
		rerender(<EpisodeTape episode={{ id: "empty", points: [] }} />);
		expect(screen.getByText("No episode observations")).toBeDefined();
		expect(container.querySelector("path")).toBeNull();
	});
});
describe("projectEpisodeTape", () => {
	it("handles flat and single observations without fabricated volatility", () => {
		const geometry = projectEpisodeTape({
			id: "single",
			points: [{ x: 5, y: 123 }],
		});
		expect(geometry?.x(5)).toBe(300);
		expect(geometry?.y(123)).toBe(130);
		expect(projectEpisodeTape({ id: "empty", points: [] })).toBeNull();
	});
});
