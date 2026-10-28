package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollapseConsecutive(t *testing.T) {
	require.Nil(t, CollapseConsecutive(nil))
	require.Equal(t, []string{"M1"}, CollapseConsecutive([]string{"M1", "M1", "M1"}))
	require.Equal(t, []string{"R01", "R02"}, CollapseConsecutive([]string{"R01", "R01", "R02"}))
	require.Equal(t, []string{"A", "B", "A"}, CollapseConsecutive([]string{"A", "B", "A"}))
}
