package svcinit

import "slices"

func sliceMap[S ~[]E, E, R any](slice S, mapper func(int, E) R) []R {
	mappedSlice := make([]R, len(slice))
	for i, v := range slice {
		mappedSlice[i] = mapper(i, v)
	}
	return mappedSlice
}

func sliceFilter[S ~[]E, E any](slice S, filter func(int, E) bool) []E {
	filteredSlice := make([]E, 0, len(slice))
	for i, v := range slice {
		if filter(i, v) {
			filteredSlice = append(filteredSlice, v)
		}
	}
	return slices.Clip(filteredSlice)
}
