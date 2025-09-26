package svcinit

import (
	"iter"
	"slices"
)

type stageTasks struct {
	tasks map[string][]*taskWrapper
}

func newStageTasks() *stageTasks {
	return &stageTasks{
		tasks: make(map[string][]*taskWrapper),
	}
}

func (s *stageTasks) add(stage string, tw *taskWrapper) {
	s.tasks[stage] = append(s.tasks[stage], tw)
}

func (s *stageTasks) stageTasks(stage string) iter.Seq[*taskWrapper] {
	return func(yield func(*taskWrapper) bool) {
		for _, t := range s.tasks[stage] {
			if !yield(t) {
				return
			}
		}
	}
}

func (s *stageTasks) stepTaskCount(step Step) (ct int) {
	for _, tasks := range s.tasks {
		for _, task := range tasks {
			if slices.Contains(taskSteps(task.task), step) {
				ct++
			}
		}
	}
	return ct
}

// stagesIter returns an interator to a list of stages.
func stagesIter(stages []string, reversed bool) iter.Seq[string] {
	if reversed {
		return reversedSlice(stages)
	} else {
		return slices.Values(stages)
	}
}
