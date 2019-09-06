package controller

import "context"

// Storage knows how to get the objects based on the ID, the Controller
// will get the object at the moment of handling the ID.
type Storage interface {
	Get(ctx context.Context, id string) (interface{}, error)
}

// StorageFunc is a helper to get a Storage from a function
type StorageFunc func(ctx context.Context, id string) (interface{}, error)

// Get satisfies Storage interface.
func (s StorageFunc) Get(ctx context.Context, id string) (interface{}, error) {
	return s(ctx, id)
}
