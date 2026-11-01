export type GateForces = {
	phase?: string;
	passed?: boolean;
	virtual?: boolean;
	reason?: string;
	ignitionHurdle?: number;
	riskTolerance?: number;
	rejectionHurdle?: number;
	netSpark?: number;
	netHold?: number;
	netRejection?: number;
	exitForce?: number;
	risk?: number;
	treeAction?: string;
	treeVirtual?: boolean;
	treeImpulse?: number;
	treeResistance?: number;
};

export type GateBadgeTone = "muted" | "up" | "warn" | "acc";

export type GateBadge = {
	label: string;
	tone: GateBadgeTone;
	hint: string;
};

/*
resolveEnterExecutionBook mirrors model.go: isVirtual := decision.Virtual || gateDec.Virtual
and desk enter blocked when !gateDec.Allowed.
*/
export function resolveEnterExecutionBook(
	gate: GateForces,
	blocked: boolean,
): "blocked" | "virtual" | "paper" | "real" {
	if (blocked) {
		return "blocked";
	}

	if (gate.virtual || gate.treeVirtual) {
		return "virtual";
	}

	if (gate.passed) {
		return "paper";
	}

	return "blocked";
}

export function formatGateBadge(input: {
	gateLive: boolean;
	phase: string;
	gate: GateForces;
	blocked: boolean;
	positionBook: "paper" | "virtual" | "real";
}): GateBadge {
	if (!input.gateLive) {
		return {
			label: "…",
			tone: "muted",
			hint: "Waiting for the next training step on this symbol.",
		};
	}

	const phase = input.phase || "hold";

	if (phase === "hold") {
		const label =
			input.positionBook === "virtual"
				? "Virtual book · open"
				: input.positionBook === "real"
					? "Real book · open"
					: "Paper book · open";
		const tone =
			input.positionBook === "virtual"
				? "acc"
				: input.positionBook === "real"
					? "warn"
					: "up";
		return {
			label,
			tone,
			hint: "You already hold this lot. Hold-phase gate bars are environmental context for exit pressure, not a new enter approval.",
		};
	}

	if (phase === "exit") {
		if (input.blocked || input.gate.passed === false) {
			return {
				label: "Cognitive exit blocked",
				tone: "warn",
				hint: "Profit exit needs exit trie, in-profit mark, and gate clearance.",
			};
		}
		return {
			label: "Cognitive exit allowed",
			tone: "up",
			hint: "Gate would allow a cognitive harvest exit on this tick.",
		};
	}

	const enterBook = resolveEnterExecutionBook(input.gate, input.blocked);
	switch (enterBook) {
	case "blocked":
		return {
			label: "Enter blocked",
			tone: "warn",
			hint: "Gate or invariants veto a new lot on this tick.",
		};
	case "virtual":
		return {
			label: "Enter · virtual explore",
			tone: "acc",
			hint: "Resistance still above the paper floor — exploring on the virtual ledger.",
		};
	case "real":
		return {
			label: "Enter · real armed",
			tone: "warn",
			hint: "Paper edge is armed — live wallet may open on this validated path.",
		};
	default:
		return {
			label: "Enter · paper eligible",
			tone: "up",
			hint: "Path resistance is at or below the paper floor — validated paper enter.",
		};
	}
}

export function formatTrieLine(gate: GateForces, phase: string): string {
	const action = gate.treeAction ?? "—";
	const impulse = gate.treeImpulse ?? 0;
	const resistance = gate.treeResistance ?? 1;
	const clears = impulse >= resistance;
	const phaseLabel = phase || "hold";

	if (phaseLabel === "enter") {
		const book = gate.treeVirtual ? "virtual explore" : "paper-eligible";
		return `Enter trie · ${action} · impulse ${impulse.toFixed(2)} vs resistance ${resistance.toFixed(2)} (${clears ? "clears" : "below"} book → ${book})`;
	}

	return `${phaseLabel} trie · ${action} · impulse ${impulse.toFixed(2)} vs resistance ${resistance.toFixed(2)}`;
}

export function gateBadgeClassName(tone: GateBadgeTone): string {
	switch (tone) {
	case "up":
		return "text-(--up)";
	case "warn":
		return "text-(--warn)";
	case "acc":
		return "text-(--acc)";
	default:
		return "text-(--f4)";
	}
}
