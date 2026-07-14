import { bench, describe } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { signalsSurfaceSources } from "./signals-surface";

describe("signalsSurfaceSources", () => {
	bench("merges configured kernels with backend sources", () => {
		signalsSurfaceSources(DEFAULT_KERNELS, ["customflow", "customregime"]);
	});
});
