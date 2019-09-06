package controller

import (
	"context"
	"errors"
)

// EventKind is the kind of event the Watcher will return.
type EventKind string

const (
	EventAdded    EventKind = "ADDED"
	EventModified EventKind = "MODIFIED"
	EventDeleted  EventKind = "DELETED"
	EventError    EventKind = "ERROR"
)

// Event represents a event from a object for the lister watchers.
type Event struct {
	ID   string
	Kind EventKind
}

// ListerWatcher knows how to list object IDs and subscribe using watchers. The List
// will be triggered at regular intervals (reconciliation loop/pattern), the watchers
// are events that will be received, these are for real time and an optimization
// so it's not needed to wait for the next reconcile iteration to handle objects.
//
// The controller will use the IDs for everything but the handler will get the object
// using a Store.
type ListerWatcher interface {
	// List returns a list of all the IDs that want to be processed.
	List(ctx context.Context) (ids []string, err error)
	// Watch returns a channel that will return the object changes to be processed
	// as an optimization to not wait for the next List iteration.
	Watch(ctx context.Context) (<-chan Event, error)
}

// ListerWatcherFunc is a helper type to get a ListerWatcher using
// directly functions in a struct.
type ListerWatcherFunc struct {
	ListFunc  func(ctx context.Context) (ids []string, err error)
	WatchFunc func(ctx context.Context) (<-chan Event, error)
}

// List satisfies ListerWatcher interface.
func (l ListerWatcherFunc) List(ctx context.Context) ([]string, error) {
	if l.WatchFunc == nil {
		return nil, errors.New("list function can't be nil")
	}
	return l.ListFunc(ctx)
}

// Watch satisfies ListerWatcher interface.
func (l ListerWatcherFunc) Watch(ctx context.Context) (<-chan Event, error) {
	if l.WatchFunc == nil {
		return nil, errors.New("watch function can't be nil")
	}
	return l.WatchFunc(ctx)
}
