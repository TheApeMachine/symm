import { bench, describe } from "vitest";
import { projectEpisodeTape } from "./episode-tape";
// Fixture: 1,000 observations with alternating multi-leg movements.
const episode = {
	id: "benchmark",
	points: Array.from({ length: 1000 }, (_, index) => ({
		x: index,
		y: 100 + (index % 40 < 20 ? index % 20 : 20 - (index % 20)),
	})),
};
describe("projectEpisodeTape", () => {
	bench("projects 1000 supplied observations", () => {
		projectEpisodeTape(episode);
	});
});
