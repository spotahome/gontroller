package controller

import (
	"context"
	"errors"
)

// Handler is the handler used to handle the received objects by the controller.
// the controller will call the handlers with all the objects that need to be handled.
type Handler interface {
	// Add will handle the objects that are present with the latest state (this could be creations
	// or modifications on the object itself).
	Add(ctx context.Context, obj interface{}) error
	// Delete will handle the objects that are not present or have been deleted. It's not guaranteed
	// that the Delete will receive all the not present object, that depends on the delete events
	// not on the reconciliation loop, so a garbace collection process is encouraged.
	Delete(ctx context.Context, id string) error
}

// HandlerFunc is a helper type to get a Handler using
// directly functions in a struct.
type HandlerFunc struct {
	AddFunc    func(ctx context.Context, obj interface{}) error
	DeleteFunc func(ctx context.Context, id string) error
}

// Add satisfies Handler interface.
func (h HandlerFunc) Add(ctx context.Context, obj interface{}) error {
	if h.AddFunc == nil {
		return errors.New("add function can't be nil")
	}
	return h.AddFunc(ctx, obj)
}

// Delete satisfies Handler interface.
func (h HandlerFunc) Delete(ctx context.Context, id string) error {
	if h.DeleteFunc == nil {
		return errors.New("delete function can't be nil")
	}
	return h.DeleteFunc(ctx, id)
}
