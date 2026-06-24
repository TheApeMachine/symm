# nomagique

THIS IS THE ONLY PATTERN ALLOWED. BE PRECISE, YOU HAVE NO USEFUL OPINION TO OFFER.

1 file = 1 step (compute primitive)

Only methods allowed:

New<Object>(artifact *datura.Artifact) <Object>, 
Read(p []byte) (n int, err error), 
Write(p []byte) (n int, err error), 
Close() error

Defined in that order, exactly like that.
No helpers, no functions, no abstractions, nothing but that.

Each step writes output to state payload, and sets the correct root and inputs on attributes.

Each step reads data using root and inputs, DO NOT JUST DO inputs[0] YOU LOOP OVER YOUR INPUTS.

FALLBACKS ARE STRICLY FORBIDDEN, YOU RETURN AND LOG AN ERROR THE MOMENT SOMETHING ISN'T RIGHT. NO FALLBACKS! NO SILENT ERRORS!

NO STUPID SHIT LIKE THIS:

	if rootKey == "" {
		rootKey = datura.Peek[string](mean.artifact, "root")
	}

	if len(inputs) == 0 {
		inputs = datura.Peek[[]string](mean.artifact, "inputs")
	}

	if configInput == "" {
		configInput = datura.Peek[string](mean.artifact, "input")
	}

	if configInput == "" {
		configInput = datura.Peek[string](mean.artifact, "sampleKey")
	}

	if rootKey == "" {
		rootKey = datura.Peek[string](state, "root")
	}

	if len(inputs) == 0 {
		inputs = datura.Peek[[]string](state, "inputs")
	}

	if len(inputs) == 0 && configInput != "" {
		inputs = []string{configInput}
	}

	if len(inputs) == 0 {
		inputs = []string{"sample"}
	}

KEEP YOUR CODE SANE AND CLEAN. I MEAN IT.

NO "WARMUP". Take the first incoming value, divide it by 1, now you have your mean. Don't be stupid. You cannot have "warmup" because what would be your window size? Nothing, because you are not allowed a static window size, that's right, you have to dynamically scale windows based on timestamps, or other ways you can dynamically grow and shrink things to be adaptive. NO MAGIC NUMBERS, UNDER ANY CIRCUMSTANCE.

Tests mirror file naming and method naming and use nested BDD cases.

Make use of `max` `min` `math.Abs` etc. instead of using `if` statements where possible. Always look for ways to write the same functionality in a more compact way, by which I do NOT mean, helpers, functions, etc. I mean the simple, super low-hanging fruit where:

	if longWindow < 1 {
		longWindow = 1
	}

is just unneeded. Write better code!