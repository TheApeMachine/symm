export type Envelope = {
	trade: Trade;
	correlation: Record<string, CohortOutput>;
	cvd: Record<string, FlowOutput>;
	depthflow: Record<string, BookflowOutput>;
	hawkes: Record<string, ExcitationOutcome>;
	leadlag: Record<string, LagOutcome>;
	liquidity: Record<string, DepthOutput>;
	morphology: Record<string, MorphologyOutput>;
	pumpdump: Record<string, IgnitionOutput>;
	sentiment: Record<string, ConvictionOutput>;
	toxicity: Record<string, BookQualityOutput>;
	resonance: Resonance;
	regions: Record<string, Region[]>;
	paths: Paths;
	decisions: Record<string, Decision>;
};

export type Decision = {
	symbol: string;
	action: string;
	reason: string;
	gate?: string;
	path?: string;
	virtual?: boolean;
	confidence?: number;
};

export type LightTick = {
	epoch: number;
	n: number;
	symbols: string[];
	paths: Record<string, string>;
	decisions: Record<string, Decision>;
};

export type Trade = {
	symbol: string;
	price: number;
	quantity: number;
	side: "buy" | "sell";
	timestamp: number;
};

export type CohortOutput = {
	correlation: number;
	energy: number;
	category: number;
	strength: number;
	eligible: boolean;
	herdScore: number;
	alphaScore: number;
	noiseScore: number;
	stressScore: number;
	peakScore: number;
};

export type FlowOutput = {
	value: number;
	absorption: number;
	drive: number;
	balance: number;
	starvation: number;
	net: number;
	netFraction: number;
	category: number;
};

export type BookflowOutput = {
	value: number;
	hypothesisSeparation: number;
	loadedScore: number;
	spoofScore: number;
	thinScore: number;
	neutralScore: number;
	category: number;
};

export type ExcitationOutcome = {
	fit: BivariateFit;
	observedFrom: number;
	at: number;
	horizon: number;
	fitObservedFrom: number;
	fitAt: number;
	hawkesPoissonLogLikelihoodDelta: number;
	crossSelfLogLikelihoodDelta: number;
};

export type BivariateFit = {
	muX: number;
	muY: number;
	alphaXX: number;
	alphaXY: number;
	alphaYX: number;
	alphaYY: number;
	beta: number;
};

export type LagOutcome = {
	inefficientScore: number;
	syncScore: number;
	decoupledScore: number;
	stallScore: number;
	strength: number;
	category: number;
	eligible: boolean;
	price: number;
};

export type DepthOutput = {
	value: number;
	scarcityScore: number;
	medianScore: number;
	depthScore: number;
	strength: number;
	category: number;
};

export type MorphologyOutput = {
	value: number;
	shapeStable: number;
	askVacuum: number;
	bidNotionalLean: number;
	strength: number;
};

export type IgnitionOutput = {
	value: number;
	hypothesisSeparation: number;
	loadedScore: number;
	spoofScore: number;
	thinScore: number;
	neutralScore: number;
	category: number;
	ready: boolean;
};

export type ConvictionOutput = {
	value: number;
	surgeScore: number;
	divergentScore: number;
	slumpScore: number;
	strength: number;
	category: number;
	breadth: number;
	change: number;
};

export type BookQualityOutput = {
	value: number;
	bluffScore: number;
	vacuumScore: number;
	supportScore: number;
	strength: number;
	category: number;
	price: number;
};

export type Resonance = Record<string, Record<string, number>>;

export type Paths = Record<string, Tokens>;

export type Region = {
	symbol: string;
	key: number;
	deformation: number;
	brightness: number;
	counts: number;
};

export type Tokens = Record<string, Token>;

export type Token = {
	symbol: string;
	price: number;
	quantity: number;
};
