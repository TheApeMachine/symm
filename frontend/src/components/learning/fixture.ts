import { LearningStateT } from "#/providers/telemetry/telemetry/learning-state";
import { LearningAgentT } from "#/providers/telemetry/telemetry/learning-agent";
import { LearningPriorT } from "#/providers/telemetry/telemetry/learning-prior";

// A funded account with a measured loss; wire constructors own optional fields.
export const learningFixture = () => {
	const state = new LearningStateT();
	state.atNs = 1000000000n;
	state.status = "learning";
	state.steps = 100n;
	state.decisions = 20n;
	state.resolved = 3n;
	const member = new LearningAgentT();
	Object.assign(member, {
		initial: "200",
		cash: "190",
		equity: "198",
		profit: "-2",
		fees: "1",
		realized: "-1",
		unrealized: "-1",
		wealth: -0.01,
		decisions: 20n,
		fills: 4n,
		pending: 17n,
		wins: 1n,
		losses: 2n,
		status: "learning",
	});
	member.reading = new LearningPriorT();
	Object.assign(member.reading, {
		defined: true,
		samples: 3n,
		mean: -0.005,
		support: 2.5,
	});
	state.agents = [member];
	return state;
};
