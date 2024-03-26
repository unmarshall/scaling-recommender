package virtualenv

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
)

type EventControl interface {
	ListEvents(ctx context.Context, filters ...common.EventFilter) ([]corev1.Event, error)
}

func NewEventControl(cl client.Client) EventControl {
	return &eventControl{
		client: cl,
	}
}

type eventControl struct {
	client client.Client
}

func (e eventControl) ListEvents(ctx context.Context, filters ...common.EventFilter) ([]corev1.Event, error) {
	eventList := &corev1.EventList{}
	if err := e.client.List(ctx, eventList); err != nil {
		return nil, err
	}
	if filters == nil {
		return eventList.Items, nil
	}
	filteredEvents := make([]corev1.Event, 0, len(eventList.Items))
	for _, event := range eventList.Items {
		if ok := evaluateFilters(&event, filters); ok {
			filteredEvents = append(filteredEvents, event)
		}
	}
	return filteredEvents, nil
}

func evaluateFilters(event *corev1.Event, filters []common.EventFilter) bool {
	for _, f := range filters {
		if ok := f(event); !ok {
			return false
		}
	}
	return true
}
