# do-it-right

THE OVERALL SYSTEM WORKS LIKE THIS:

1. Signals convert raw market data into Measurement
   Each Signal MUST emit a Measurement on each Tick, which includes a CONFIDENCE, and SNR (SURPRISE)
   Each Signal decides for themselves which asset pairs to subscribe to.
2. Measurements are picked up by the Story
   The Story uses Measurements to run a decision Tree and Branches, which can results in an Action
3. The trader acts upon those Actions via the broker Desk
4. The paper websocket emulation takes care of everything that is different between paper and live trading

WHAT YOU SHOULD NOT DO:

1. Do NOT create frivolous files, helper methods, or abstractions
2. Do NOT ignore the rules in @AGENTS.md
3. Do NOT hide flaws, silence errors (always use errnie.Error(err)), or use silent fallbacks
4. Do NOT change any code that is not part of what you are solving, your opinion is not useful
5. Do NOT restore code from git, also not by memory, always look forwards not backwards

WHAT YOU SHOULD DO:

1. Let the system fail and halt immediately the moment something isn't as it is supposed to be
2. Provide the minimal amount of changes for a correct solution
3. Build on the existing code, and provide your best quality solutions
4. Make sure your tests are highly relevant, useful, and test edge conditions and adverserial scenarios
5. Verify everything is working before you deliver

COMPLEXITY IS EARNED, DO NOT UNDER ANY CIRCUMSTANCE INTRODUCE MORE THAN ABSOLUTELY NEEDED!