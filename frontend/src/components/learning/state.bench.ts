import { bench } from "vitest";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

const source = learningFixture();
bench("projectLearning signed account", () => projectLearning(source, ""));
