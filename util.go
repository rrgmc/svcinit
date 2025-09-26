package svcinit

import (
	"fmt"
	"iter"
	"slices"
	"strings"
)

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

func stringerString[T fmt.Stringer](s []T) string {
	return strings.Join(stringerList(s), ",")
}

func stringerList[T fmt.Stringer](s []T) []string {
	ss := make([]string, len(s))
	for i, v := range s {
		ss[i] = v.String()
	}
	return ss
}

func stringerIter[T fmt.Stringer](s iter.Seq[T]) []string {
	var ss []string
	for v := range s {
		ss = append(ss, v.String())
	}
	return ss
}
