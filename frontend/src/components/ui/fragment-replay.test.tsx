import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { FragmentReplay } from "./fragment-replay";

describe("FragmentReplay", () => {
	it("renders saved metric cursor, all four anchors, and causal region history", () => {
		const html = renderToStaticMarkup(<FragmentReplay observation={{capture:{session:"1789999999999999999",endpoint:"metrics"},cursor:{sequence:37,record:0},symbol:"BTC/USD",event:{a:{sequence:30},b:{sequence:34},c:{sequence:50},d:{sequence:60},excursion:-0.012}}} context={{symbol:"BTC/USD",vocabulary:"grid-v1",tokens:["region4"],history:["region2","region4"],epoch:"1789999999999999999",sequence:"37"}} completed="128" />);
		for (const value of ["1789999999999999999","37","30","34","50","60","region2","region4","grid-v1","-1.2000%"]) expect(html).toContain(value);
		expect(html).not.toContain("price");
	});
});
