// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StageDiagnostics, StageRow } from "./stage-diagnostics";

afterEach(cleanup);

describe("StageDiagnostics", () => {
	it("keeps active consumers visible beside idle downstream consumers", () => {
		render(
			<StageDiagnostics
				readiness={{
					phase: "WARMING_SIGNALS",
					ready: false,
					contributing: 1,
					total: 2,
					missing: ["correlation"],
					input_count: 3,
				}}
			>
				<StageRow
					name="spot"
					group="feeds"
					barrier={0}
					completed="9007199254740993"
					epoch="1789999999999999999"
					sequence="94"
				/>
				<StageRow name="training" group="learning" barrier={7} />
			</StageDiagnostics>,
		);
		expect(screen.getByText("9007199254740993")).toBeDefined();
		expect(screen.getByText("1789999999999999999")).toBeDefined();
		expect(screen.getByText("training")).toBeDefined();
		expect(screen.getByText("Incomplete families: correlation")).toBeDefined();
		expect(screen.getAllByText("—")).toHaveLength(3);
	});
});
