package logic

import "fmt"

const CategoryNoneIndex = 0

/*
RequireRealCategoryIndex validates that index refers to a real category slot.
All real categories use 1..categoryCount; index 0 is reserved for none/winner sentinel.
*/
func RequireRealCategoryIndex(index int, categoryCount int) error {
	if categoryCount <= 0 {
		return fmt.Errorf("logic: category count must be positive, got %d", categoryCount)
	}

	if index < 1 || index > categoryCount {
		return fmt.Errorf(
			"logic: real category index must be in [1,%d], got %d",
			categoryCount,
			index,
		)
	}

	return nil
}

/*
RealCategoryIndex returns the 1-based transition index for a category type.
Returns CategoryNoneIndex when the category is unset or unknown.
*/
func RealCategoryIndex(category CategoryType, mapping map[CategoryType]int) int {
	index, ok := mapping[category]

	if !ok || index < 1 {
		return CategoryNoneIndex
	}

	return index
}
