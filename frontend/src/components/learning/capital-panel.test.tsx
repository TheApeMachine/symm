import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CapitalPanel } from "./capital-panel";
import type { AccountLearning, LearningView } from "./state";

const account: AccountLearning = {
	aborted: 2,
	pendingState: "awaiting execution",
	horizonNs: 5000000000,
	horizonSource: "earliest viable candidate expiry",
	state: {
		mark: {
			at: "2026-09-06T00:00:00Z",
			version: 3,
			equity: 202,
			hasFunding: true,
			netFunding: 0,
		},
		cash: "50",
		actualCash: "150",
		committed: "100",
		positions: { "BTC/USD": "0.002" },
		complete: true,
	},
	outcome: { totalReward: 2, rate: 4, hasRate: true },
	target: 0.02,
	resolved: 1,
	mfe: 2,
	mae: -1,
	timeToPositiveNs: 500000000,
	timeToBreakevenNs: 500000000,
	holdingNs: 1000000000,
	trajectory: [],
	pending: "allocation-1",
};
const view = {
	skill: { mode: "learning" },
	realizationAllowed: true,
	authorizedMode: "learning",
	execution: { refused: 2, lastRefusal: "insufficient capital" },
	capital: {
		choice: { symbol: "", kind: "hold", power: 0 },
		prior: { Defined: false },
		decisions: 1,
		demand: "300",
		actual: account,
		exploration: account,
		candidates: [],
		outcomes: [],
	},
} as unknown as LearningView;

describe("CapitalPanel", () => {
	it("shows separate evidence sources and an aborted allocation without completed evidence", () => {
		const prior = {
			...view.capital!.prior,
			Defined: true,
			VarianceDefined: false,
			Mean: 0.01,
			Support: 3,
			Samples: 3,
			Depth: 2,
			ContextLength: 5,
			Authority: 1,
		};
		const evidence = {
			scope: "global",
			global: prior,
			symbol: prior,
			selected: prior,
		};
		const html = renderToStaticMarkup(
			<CapitalPanel
				view={{
					...view,
					capital: {
						...view.capital!,
						prior,
						warmupUnverified: 4,
						evidence: {
							source: "capital_account",
							scope: "global",
							virtual: evidence,
							actual: evidence,
							selected: prior,
						},
						actual: {
							...account,
							resolved: 0,
							pending: "",
							execution: {
								state: "aborted",
								at: account.state.mark.at,
								detail: "repricing failed",
							},
						},
					},
				}}
			/>,
		);
		expect(html).toContain("Selected evidence: capital_account");
		expect(html).toContain("Virtual allocation evidence");
		expect(html).toContain("Actual allocation evidence");
		expect(html).toContain("Aborted allocations 2");
		expect(html).toContain("repricing failed");
		expect(html).toContain("allocation target unresolved");
		expect(html).toContain("depth 2/5");
		expect(html).toContain("samples 3");
		expect(html).not.toContain("samples 6");
		expect(html).toContain("earliest viable candidate expiry");
	});
	it("renders producer-shaped lowercase reward fields and separate authority", () => {
		const html = renderToStaticMarkup(<CapitalPanel view={view} />);
		expect(html).toContain("Wallet profit 2");
		expect(html).toContain("rate 4/s");
		expect(html).toContain("reserved 100");
		expect(html).toContain("Skill allows increase: no");
		expect(html).toContain("Realization allows increase: yes");
		expect(html).toContain("Genuine reductions allowed");
		expect(html).toContain("wait");
		expect(html).toContain("Actual account teacher");
		expect(html).toContain("One shared exploration wallet");
		expect(html).not.toContain("NaN");
	});
	it("preserves unavailable and unresolved states instead of fabricating zero profit", () => {
		const unknown = {
			...account,
			state: {
				...account.state,
				complete: false,
				reason: "funding unavailable",
				mark: { ...account.state.mark, version: 0 },
			},
			resolved: 0,
		};
		const html = renderToStaticMarkup(
			<CapitalPanel
				view={{
					...view,
					capital: { ...view.capital!, actual: unknown, exploration: unknown },
				}}
			/>,
		);
		expect(html).toContain("funding unavailable");
		expect(html).toContain("Wallet profit unavailable");
		expect(html).toContain("allocation target unresolved");
		expect(renderToStaticMarkup(<CapitalPanel view={null} />)).toContain(
			"Awaiting shared-capital state",
		);
	});
});
