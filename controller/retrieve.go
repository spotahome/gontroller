package controller

import "errors"

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
	List() (ids []string, err error)
	// Watch returns a channel that will return the object changes to be processed
	// as an optimization to not wait for the next List iteration.
	Watch() (<-chan Event, error)
}

// ListerWatcherFunc is a helper type to get a ListerWatcher using
// directly functions in a struct.
type ListerWatcherFunc struct {
	ListFunc  func() (ids []string, err error)
	WatchFunc func() (<-chan Event, error)
}

// List satisfies ListerWatcher interface.
func (l ListerWatcherFunc) List() ([]string, error) {
	if l.WatchFunc == nil {
		return nil, errors.New("list function can't be nil")
	}
	return l.ListFunc()
}

// Watch satisfies ListerWatcher interface.
func (l ListerWatcherFunc) Watch() (<-chan Event, error) {
	if l.WatchFunc == nil {
		return nil, errors.New("watch function can't be nil")
	}
	return l.WatchFunc()
}
