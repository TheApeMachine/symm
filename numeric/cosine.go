package numeric

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

/*
CosineSparseMaps is cosine similarity between sparse count maps (e.g. co-occurrence rows).
*/
func CosineSparseMaps(left map[string]float64, right map[string]float64) float64 {
	if left == nil || right == nil {
		return 0
	}

	dot := 0.0
	leftMag := 0.0
	rightMag := 0.0

	for token, count := range left {
		dot += count * right[token]
		leftMag += count * count
	}

	for _, count := range right {
		rightMag += count * count
	}

	if leftMag == 0 || rightMag == 0 {
		return 0
	}

	return dot / (math.Sqrt(leftMag) * math.Sqrt(rightMag))
}

/*
CharacterNgramCosine is cosine similarity over character n-gram counts (^/$ padded).
ASCII inputs with n<=8 use byte-packed uint64 keys to avoid per-gram string allocs.
*/
func CharacterNgramCosine(left string, right string, n int) (float64, error) {
	if n <= 1 {
		return 0, fmt.Errorf("numeric: CharacterNgramCosine n must be >= 2, got %d", n)
	}

	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	if left == "" || right == "" {
		return 0, nil
	}

	if n <= 8 && isAllASCII(left) && isAllASCII(right) {
		return characterNgramCosineASCII(left, right, n), nil
	}

	leftRunes := []rune("^" + left + "$")
	rightRunes := []rune("^" + right + "$")

	if len(leftRunes) < n || len(rightRunes) < n {
		return 0, nil
	}

	leftCap := len(leftRunes) - n + 1
	rightCap := len(rightRunes) - n + 1

	leftCounts := make(map[string]float64, leftCap)
	rightCounts := make(map[string]float64, rightCap)

	for offset := 0; offset <= len(leftRunes)-n; offset++ {
		leftCounts[string(leftRunes[offset:offset+n])]++
	}

	for offset := 0; offset <= len(rightRunes)-n; offset++ {
		rightCounts[string(rightRunes[offset:offset+n])]++
	}

	dot := 0.0
	leftMag := 0.0
	rightMag := 0.0

	for gram, count := range leftCounts {
		dot += count * rightCounts[gram]
		leftMag += count * count
	}

	for _, count := range rightCounts {
		rightMag += count * count
	}

	if leftMag == 0 || rightMag == 0 {
		return 0, nil
	}

	return dot / (math.Sqrt(leftMag) * math.Sqrt(rightMag)), nil
}

func isAllASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}

func packByteNGram(b []byte, start, n int) uint64 {
	var k uint64

	for j := 0; j < n; j++ {
		k = k<<8 | uint64(b[start+j])
	}

	return k
}

func characterNgramCosineASCII(left, right string, n int) float64 {
	lb := make([]byte, 0, len(left)+2)
	lb = append(lb, '^')
	lb = append(lb, left...)
	lb = append(lb, '$')

	rb := make([]byte, 0, len(right)+2)
	rb = append(rb, '^')
	rb = append(rb, right...)
	rb = append(rb, '$')

	if len(lb) < n || len(rb) < n {
		return 0
	}

	leftCounts := make(map[uint64]float64, len(lb)-n+1)
	rightCounts := make(map[uint64]float64, len(rb)-n+1)

	for offset := 0; offset <= len(lb)-n; offset++ {
		leftCounts[packByteNGram(lb, offset, n)]++
	}

	for offset := 0; offset <= len(rb)-n; offset++ {
		rightCounts[packByteNGram(rb, offset, n)]++
	}

	dot := 0.0
	leftMag := 0.0
	rightMag := 0.0

	for gram, count := range leftCounts {
		dot += count * rightCounts[gram]
		leftMag += count * count
	}

	for _, count := range rightCounts {
		rightMag += count * count
	}

	if leftMag == 0 || rightMag == 0 {
		return 0
	}

	return dot / (math.Sqrt(leftMag) * math.Sqrt(rightMag))
}
