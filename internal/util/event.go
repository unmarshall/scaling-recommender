package util

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

// WaitForAndRecordPodSchedulingEvents watches for pod scheduling events and returns the names of the pods that have been scheduled and unscheduled.
func WaitForAndRecordPodSchedulingEvents(ctx context.Context, ec virtualenv.EventControl, since time.Time, pods []corev1.Pod, timeout time.Duration) (scheduledPodNames sets.Set[string], unscheduledPodNames sets.Set[string], err error) {
	tick := time.NewTicker(timeout)
	defer tick.Stop()
	pollTick := time.NewTicker(100 * time.Millisecond)
	defer pollTick.Stop()

	podNames := GetPodNames(pods)
	scheduledPodNames = sets.New[string]()
	unscheduledPodNames = sets.New[string]()

loop:
	for {
		select {
		case <-ctx.Done():
			return scheduledPodNames, unscheduledPodNames, fmt.Errorf("context cancelled, timeout waiting for pod events: %w", ctx.Err())
		case <-tick.C:
			return scheduledPodNames, unscheduledPodNames, fmt.Errorf("timeout waiting for pod events")
		case <-pollTick.C:
			events, err := ec.ListEvents(ctx, filterEventBeforeTimeForPods(since, podNames))
			if err != nil {
				slog.Error("cannot get pod scheduling events, this will be retried", "error", err)
			}
			for _, event := range events {
				switch event.Reason {
				case "FailedScheduling":
					unscheduledPodNames.Insert(event.InvolvedObject.Name)
				case "Scheduled":
					scheduledPodNames.Insert(event.InvolvedObject.Name)
					podNames = slices.DeleteFunc(podNames, func(podName string) bool {
						return podName == event.InvolvedObject.Name
					})
					unscheduledPodNames.Delete(event.InvolvedObject.Name)
				}
			}
			if len(scheduledPodNames)+len(unscheduledPodNames) == len(pods) {
				break loop
			}
		}
	}
	return scheduledPodNames, unscheduledPodNames, nil
}

// filterEventBeforeTimeForPods returns an EventFilter that filters events that occurred before the given time and are related to the given pods.
func filterEventBeforeTimeForPods(since time.Time, targetPodNames []string) common.EventFilter {
	return func(event *corev1.Event) bool {
		if event.EventTime.BeforeTime(&metav1.Time{Time: since}) {
			return false
		}
		return slices.Contains(targetPodNames, event.InvolvedObject.Name)
	}
}
