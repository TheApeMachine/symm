import { bench, describe } from "vitest";
import { SIGNALS } from "#/collections/app";
import { signalsSurfaceSources } from "./signals-surface";

describe("signalsSurfaceSources", () => {
	bench("merges configured kernels with backend sources", () => {
		signalsSurfaceSources(SIGNALS, ["customflow", "customregime"]);
	});
});
