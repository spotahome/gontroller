package metrics

import "time"

// HandleKind is the kind of handler call on the controller.
type HandleKind string

const (
	// AddHandleKind represents the `Add` Handler call.
	AddHandleKind HandleKind = "add"
	// DeleteHandleKind represents the `Delete` Handler call.
	DeleteHandleKind HandleKind = "delete"
)

// Recorder is the metrics service that knows how to record the metrics
// of the controller.
type Recorder interface {
	// WithID will set the ID name to the recorder and every metric
	// measured with the obtained recorder will be identified with
	// the name.
	WithID(id string) Recorder
	// IncControllerQueuedTotal increments the total number of objects queued on the controller queue.
	IncControllerQueuedTotal()
	// ObserveControllerOnQueueLatency will measure the duration the object has been queued
	// before being handled and this means the object just been dequeued.
	ObserveControllerOnQueueLatency(start time.Time)
	// ObserveControllerStorageGetLatency will measure the duration getting the object
	// from the storage.
	ObserveControllerStorageGetLatency(start time.Time, success bool)
	// ObserveControllerListLatency will measure the duration getting the list of object IDs
	// using the listerwatcher.
	ObserveControllerListLatency(start time.Time, success bool)
	// ObserveControllerHandleLatency will measure the duration to handle a object.
	ObserveControllerHandleLatency(start time.Time, kind HandleKind, success bool)
}
