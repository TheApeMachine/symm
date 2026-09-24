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
			"M16,138",
		);
		expect(screen.getByText("A")).toBeDefined();
		expect(JSON.stringify(episode)).toBe(before);
		rerender(<EpisodeTape episode={{ id: "empty", points: [] }} />);
		expect(screen.getByText("NO EPISODE OBSERVATIONS")).toBeDefined();
		expect(container.querySelector("path")).toBeNull();
	});
});
describe("EpisodeTape marks", () => {
	it("draws hindsight marks and decisions only inside the observed run", () => {
		const episode = {
			id: "run",
			points: [
				{ x: 10, y: 1 },
				{ x: 20, y: 2 },
				{ x: 30, y: 3 },
			],
		};
		render(
			<EpisodeTape
				episode={episode}
				run={7}
				marks={{ A: 10, B: 20, C: 99 }}
				entered={20}
				exited={99}
				outcome={0.0125}
			/>,
		);
		expect(screen.getByText("EP-7")).toBeDefined();
		expect(screen.getByText("A")).toBeDefined();
		expect(screen.getByText("B")).toBeDefined();
		expect(screen.queryByText("C")).toBeNull();
		expect(screen.getByText("ENTER")).toBeDefined();
		expect(screen.queryByText("EXIT")).toBeNull();
		expect(screen.getByText("+1.25%")).toBeDefined();
	});
});
describe("projectEpisodeTape", () => {
	it("handles flat and single observations without fabricated volatility", () => {
		const geometry = projectEpisodeTape({
			id: "single",
			points: [{ x: 5, y: 123 }],
		});
		expect(geometry?.x(5)).toBe(300);
		expect(geometry?.y(123)).toBe(138);
		expect(projectEpisodeTape({ id: "empty", points: [] })).toBeNull();
	});
});
