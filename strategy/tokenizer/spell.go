package tokenizer

// CollapseConsecutive removes adjacent duplicate spell tokens. Sequitur path
// tails can repeat rule refs (M1/M1/M1); path keys should not.
func CollapseConsecutive(tokens []string) []string {
	if len(tokens) <= 1 {
		return tokens
	}
	out := make([]string, 0, len(tokens))
	out = append(out, tokens[0])
	for i := 1; i < len(tokens); i++ {
		if tokens[i] != tokens[i-1] {
			out = append(out, tokens[i])
		}
	}
	return out
}
