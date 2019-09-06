package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/spotahome/gontroller/log"
	"github.com/spotahome/gontroller/metrics"
)

// Config is the configuration of the controller.
type Config struct {
	// The name of the controller
	Name string
	// ResyncInterval is the interval the reconciliation will
	// be applied.
	ResyncInterval time.Duration
	// Workers are the number of workers the controller will have
	// handling objects.
	Workers int
	// MaxRetries is the maximum number of retries an object will have after an error.
	MaxRetries int
	// ListerWatcher is the lister watcher the controller will use to queue the elements to process.
	ListerWatcher ListerWatcher
	// Handler is the handler that will handle the elements processed by the handler.
	Handler Handler
	// Storage is the storage where the controller will get the object that the handler needs to process.
	Storage Storage
	// ObjectLocker is the locker that will be used to lock the objects that are being processed by the
	// workers.
	// By default it will use a memory lock that will ensure an object is only processed once at the
	// same time by one worker.
	ObjectLocker ObjectLocker
	// MetricsRecorder is the recorder used to record controller metrics.
	MetricsRecorder metrics.Recorder
	// Logger is the logger.
	Logger log.Logger
}

func (c *Config) defaults() error {
	if c.ResyncInterval <= 0 {
		c.ResyncInterval = 30 * time.Second
	}

	if c.Workers <= 0 {
		c.Workers = 1
	}

	if c.MaxRetries <= 0 {
		c.MaxRetries = 0
	}

	if c.Logger == nil {
		c.Logger = log.Dummy
	}

	if c.MetricsRecorder == nil {
		c.Logger.Warningf("controller metrics disabled")
		c.MetricsRecorder = metrics.Dummy
	}

	if c.Name == "" {
		c.Logger.Warningf("controller configured without name")
	}

	if c.ListerWatcher == nil {
		return fmt.Errorf("a ListerWatcher is required")
	}

	if c.Storage == nil {
		return fmt.Errorf("a Storage is required")
	}

	if c.ObjectLocker == nil {
		c.ObjectLocker = newMemoryLock()
	}

	if c.Handler == nil {
		return fmt.Errorf("a Handler is required")
	}

	return nil
}

// Controller is composed by 3 main components:
// - ListerWatcher: This piece is the one that will provide the object IDs to the
//	controller queue. Its composed by two internal pieces, the `List`, that will
//	will receive object events (create, modify, delete...).
// - Storage: The storage is the one that know how to get the object based on the
//	 ListerWatcher enqueued ID and the controller will call this store just before
//	 calling the Handler.
// - Handler: The handler will handle the `Add` (exists) and `Delete` (doesn't exist)
//   objects queued by the ListerWatcher.
//
// The controller will call the `ListerWatcher.List` method every T interval (e.g. 30s)
// to enqueue the IDs to process and the `ListerWatcher.Watch` will enqueue real time
// events to be processed (so there is no need to wait for next List iteration).
//
// The controller will be dequeueing from the queue the IDs to process them but before
// passing to the handlers it will get the object to process from the `Storage` if the
// object with the ID is not being processed already by a handler in another worker,
// this is achieved with the ObjectLocker component. After getting the object it will
// send the object  to the workers workers to handle it using the `Handler`.
type Controller struct {
	cfg          Config
	lw           ListerWatcher
	handler      Handler
	storage      Storage
	logger       log.Logger
	objectLocker ObjectLocker
	queue        chan string
	mRecorder    metrics.Recorder
	objectCache  *queuedObjectsCache
}

// New returns a new controller.
func New(cfg Config) (*Controller, error) {
	err := cfg.defaults()
	if err != nil {
		return nil, fmt.Errorf("error creating controller: %s", err)
	}

	return &Controller{
		cfg:          cfg,
		lw:           cfg.ListerWatcher,
		handler:      cfg.Handler,
		storage:      cfg.Storage,
		mRecorder:    cfg.MetricsRecorder.WithID(cfg.Name),
		logger:       cfg.Logger,
		queue:        make(chan string),
		objectLocker: cfg.ObjectLocker,
		objectCache:  newQueuedObjectsCache(),
	}, nil
}

// Run will run the controller and start handling the objects. The way this
// controller is implemented and designed is based on Kubernetes controllers although
// it isn't based on Kubernetes objects. It's a blocking action.
// The controller will end when the context is finished.
func (c *Controller) Run(ctx context.Context) error {
	// Run the enqueuers, these are the reconciliation loop (list) and the
	// event stream (the watcher).
	go c.reconciliationLoop(ctx)
	eventStream, err := c.lw.Watch(ctx)
	if err != nil {
		return err
	}
	go c.handleWatcherEvents(ctx, eventStream)

	// Run workers.
	for i := 0; i < c.cfg.Workers; i++ {
		go c.runWorker(ctx)
	}

	<-ctx.Done()
	c.logger.Infof("stopping controller...")
	return nil
}

// reconciliationLoop will  be reconciling the state in a loop
// so we base on state and not only on events, we get the design
// from Kubernetes controllers.
func (c *Controller) reconciliationLoop(ctx context.Context) {
	// Start the reconciliation loop.
	for {
		c.reconcile()

		select {
		case <-ctx.Done():
			return
		case <-time.After(c.cfg.ResyncInterval):
		}
	}
}

// reconcile will list and enqueue the required objects again.
func (c *Controller) reconcile() {
	c.logger.Debugf("reconciliation loop iteration started")

	// List all the required objects
	startList := time.Now()
	objectIDs, err := c.lw.List(context.TODO())
	c.mRecorder.ObserveControllerListLatency(startList, err == nil)
	if err != nil {
		c.logger.Errorf("error listing objects: %s", err)
		return
	}

	// Queue the objects to be handled.
	for _, id := range objectIDs {
		// Listed objects enqueue as present.
		c.enqueueObjectWithState(id, objectStatusPresent)
	}
}

func (c *Controller) handleWatcherEvents(ctx context.Context, eventStream <-chan Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-eventStream:
			switch ev.Kind {
			case EventAdded, EventModified:
				c.enqueueObjectWithState(ev.ID, objectStatusPresent)
			case EventDeleted:
				c.enqueueObjectWithState(ev.ID, objectStatusMissing)
			case EventError:
				c.logger.Errorf("object event stream error for ID %s", ev.ID)
			default:
				c.logger.Errorf("unknown object event received on stream for ID %s", ev.ID)
			}
		}
	}
}

func (c *Controller) runWorker(ctx context.Context) {
	c.logger.Debugf("worker started")

	for {
		select {
		case <-ctx.Done():
			return
		case objectID := <-c.queue:
			newCtx := context.Background()
			// process the object.
			err := c.processObject(newCtx, objectID)
			// Retry if required based on the result.
			c.retryIfRequired(objectID, err)
		}
	}
}

func (c *Controller) retryIfRequired(id string, err error) {
	retries, ok := c.objectCache.getRetries(id)
	if err != nil && ok && retries < c.cfg.MaxRetries {
		// Enqueue again for a new retry.
		c.logger.Warningf("retrying (%d/%d) due to an error processing object %s: %s", retries+1, c.cfg.MaxRetries, id, err)
		c.objectCache.incRetry(id)
		c.enqueueObject(id)
		return
	}

	// If no retries we need to remove the object from the cache to allow ListerWatcher queueing this
	// object again.
	defer c.objectCache.Delete(id)
	if err != nil {
		c.logger.Errorf("error processing object %s: %s", id, err)
	}
}

func (c *Controller) processObject(ctx context.Context, objectID string) (err error) {
	if !c.objectLocker.Acquire(objectID) {
		// Could not acquire the object processing, other
		// worker should be processing the same object.
		c.logger.Debugf("object %s locked, ignoring handling", objectID)
		return nil
	}
	defer c.objectLocker.Release(objectID)

	// What's the status of the object? We allow enqueuing when finished.
	status, ok := c.objectCache.GetStatus(objectID)
	if !ok {
		c.logger.Errorf("%s object status not present in cache", objectID)
		return nil
	}

	// Call the handler logic.
	switch status {
	case objectStatusMissing:
		// Measure handling.
		defer func(start time.Time) {
			c.mRecorder.ObserveControllerHandleLatency(start, metrics.DeleteHandleKind, err == nil)
		}(time.Now())

		return c.handler.Delete(ctx, objectID)
	case objectStatusPresent:
		// Get the object data from the storage.
		startStoreGet := time.Now()
		object, err := c.storage.Get(ctx, objectID)
		c.mRecorder.ObserveControllerStorageGetLatency(startStoreGet, err == nil)
		if err != nil {
			return err
		}

		// If no object and no error means that the store wants to ignore the
		// handling of this object.
		if object == nil {
			c.logger.Debugf("object %s has been ignored by the store", objectID)
			return nil
		}

		startHandle := time.Now()
		err = c.handler.Add(ctx, object)
		c.mRecorder.ObserveControllerHandleLatency(startHandle, metrics.AddHandleKind, err == nil)
		return err
	default:
		return fmt.Errorf("unknown object status kind: %s", status)
	}
}

// enqueueObjectWithState will enqueue the object only if is not in the queue
// if the object is already in the queue it will only update the state of the
// object on the cache.
func (c *Controller) enqueueObjectWithState(objectID string, status objectStatus) {
	// Is already queued our event? If the event is already queued
	// then don't enqueue again, we only need to update the status/
	// on the cache.
	_, ok := c.objectCache.GetStatus(objectID)

	// We need to store the state on cache before enqueuing, if we
	// don't do this, the workers could get a new object to handle
	// and they don't have the latest object information on the cache.
	c.objectCache.SetStatus(objectID, status)

	// Enqueue only if necessary. This way we don't enqueue the same
	// object to process multiple times.
	if !ok {
		c.enqueueObject(objectID)
	}
}

// enqueueObject will queue the object in the controller queue.
func (c *Controller) enqueueObject(objectID string) {
	go func() {
		c.mRecorder.IncControllerQueuedTotal()

		// When finishing this func we measure the latency of being consumed a.k.a the
		// time spent on the queue.
		defer func(start time.Time) {
			// If we are here means that the queued object has been consumed form the queue.
			c.mRecorder.ObserveControllerOnQueueLatency(start)
		}(time.Now())

		// Queue the object.
		c.queue <- objectID
	}()
}

// ObjectLocker will be used to lock the objects, this gives us the ability
// to only work with one object at the same time while we can process
// multiple objects in parallel.
type ObjectLocker interface {
	// Acquire when called it will try to acquire the lock for the
	// passed ID, this will return if the lock was acquired successfully
	// or not.
	// If the return result is true, means the object will be locked and
	// ready to be processed safely by the caller.
	Acquire(id string) bool
	// Release will release the lock if it was acquired, giving the ability
	// to be acquired again.
	Release(id string)
}

type memoryLocker struct {
	objectIDs map[string]struct{}
	mu        sync.Mutex
}

func newMemoryLock() ObjectLocker {
	return &memoryLocker{
		objectIDs: map[string]struct{}{},
	}
}

func (m *memoryLocker) Acquire(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objectIDs[id]
	if ok {
		return false
	}
	m.objectIDs[id] = struct{}{}
	return true
}
func (m *memoryLocker) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objectIDs, id)
}

// objectStatus will be used to know in what state is the object, if the
// state of the object changes while is in the queue, it will processed
// with the latest state and we will only need to process the object once.
// Example, a object X has been modified enters in the queue, but before it has
// been processed the object has been deleted, we get the object, if
// we don't have an status store we would process the object as being modified
// but the current state of the object is deleted, with this store we only
// would enqueue objectIDs and in the last moment being dequeued the object ID from
// the queue we would check the status on this object to get the latest state.
type objectStatus string

const (
	objectStatusPresent objectStatus = "present"
	objectStatusMissing objectStatus = "missing"
)

type objectData struct {
	status  objectStatus
	retries int
}

// queuedObjectsCache is a controller internal cache to store data
// about queued objects, if the object is queued it will be in this
// cache, if is not in the queue then it will not be in this cache.
type queuedObjectsCache struct {
	objects map[string]objectData
	mu      sync.Mutex
}

func newQueuedObjectsCache() *queuedObjectsCache {
	return &queuedObjectsCache{
		objects: map[string]objectData{},
	}
}

func (q *queuedObjectsCache) SetStatus(id string, st objectStatus) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.objects[id] = objectData{
		status: st,
	}
}

func (q *queuedObjectsCache) GetStatus(id string) (st objectStatus, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	d, ok := q.objects[id]
	return d.status, ok
}

func (q *queuedObjectsCache) Delete(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.objects, id)
}

func (q *queuedObjectsCache) incRetry(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	d, ok := q.objects[id]
	if !ok {
		return
	}
	d.retries++
	q.objects[id] = d
}

func (q *queuedObjectsCache) getRetries(id string) (ret int, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	d, ok := q.objects[id]
	return d.retries, ok
}
