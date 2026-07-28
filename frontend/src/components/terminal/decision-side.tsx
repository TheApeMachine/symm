let decisionsScopeSymbol: string | undefined;

/*
setDecisionsScopeSymbol pins the active candidate for decision-rail paints.
*/
export const setDecisionsScopeSymbol = (symbol: string | undefined): void => {
	decisionsScopeSymbol = symbol;
};

/*
readDecisionsScopeSymbol returns the pinned candidate scope for DRAW paints.
*/
export const readDecisionsScopeSymbol = (): string | undefined =>
	decisionsScopeSymbol;
