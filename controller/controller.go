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
}

func (c *Config) defaults() {
	if c.ResyncInterval <= 0 {
		c.ResyncInterval = 30 * time.Second
	}

	if c.Workers <= 0 {
		c.Workers = 1
	}

	if c.MaxRetries <= 0 {
		c.MaxRetries = 0
	}
}

// Controller is the main controller that will process the objects.
// The controller will have a lister watcher, a handler and a Storer.
//
// TODO: explain controller
type Controller struct {
	cfg         Config
	lw          ListerWatcher
	handler     Handler
	storage     Storage
	logger      log.Logger
	objectLock  objectLock
	queue       chan string
	metricssvc  metrics.Recorder
	objectCache *queuedObjectsCache
}

// New returns a new controller.
func New(cfg Config, lw ListerWatcher, handler Handler, storage Storage, metricssvc metrics.Recorder, logger log.Logger) *Controller {
	cfg.defaults()

	if metricssvc == nil {
		logger.Warningf("controller metrics disabled")
		metricssvc = metrics.Dummy
	}

	if cfg.Name == "" {
		logger.Warningf("controller configured without name")
	}

	return &Controller{
		cfg:         cfg,
		lw:          lw,
		handler:     handler,
		storage:     storage,
		metricssvc:  metricssvc.WithID(cfg.Name),
		logger:      logger,
		queue:       make(chan string),
		objectLock:  newMemoryLock(),
		objectCache: newQueuedObjectsCache(),
	}
}

// Run will run the controller and start handling the objects. The way this
// controller is implemented and designed is based on Kubernetes controllers although
// it isn't based on Kubernetes objects. It's a blocking action.
func (c *Controller) Run(stopC chan struct{}) error {
	// Run the enqueuers, these are the reconciliation loop (list) and the
	// event stream (the watcher).
	go c.reconciliationLoop(stopC)
	eventStream, err := c.lw.Watch()
	if err != nil {
		return err
	}
	go c.handleWatcherEvents(stopC, eventStream)

	// Run workers.
	for i := 0; i < c.cfg.Workers; i++ {
		go c.runWorker(stopC)
	}

	<-stopC
	c.logger.Infof("stop signal received, stopping controller")
	return nil
}

// reconciliationLoop will  be reconciling the state in a loop
// so we base on state and not only on events, we get the design
// from Kubernetes controllers.
func (c *Controller) reconciliationLoop(stopC chan struct{}) {
	// Run just when we start the loop and the let the time.After
	// do it's job.
	c.reconcile()

	// Start the reconciliation loop.
	for {
		select {
		case <-stopC:
			return
		case <-time.After(c.cfg.ResyncInterval):
			c.reconcile()
		}
	}
}

// reconcile will list and enqueue the required objects again.
func (c *Controller) reconcile() {
	// List all the required objects
	startList := time.Now()
	objectIDs, err := c.lw.List()
	c.metricssvc.ObserveControllerListLatency(startList, err == nil)
	if err != nil {
		c.logger.Errorf("error listing objects: %s", err)
	}

	// Queue the objects to be handled.
	for _, id := range objectIDs {
		// Listed objects enqueue as present.
		c.enqueueObjectWithState(id, objectStatusPresent)
	}
}

func (c *Controller) handleWatcherEvents(stopC chan struct{}, eventStream <-chan Event) {
	for {
		select {
		case <-stopC:
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

func (c *Controller) runWorker(stopC chan struct{}) {
	for {
		select {
		case <-stopC:
			return
		case objectID := <-c.queue:
			// process the object.
			err := c.processObject(objectID)
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

func (c *Controller) processObject(objectID string) (err error) {
	if !c.objectLock.Acquire(objectID) {
		// Could not acquire the object processing, other
		// worker should be processing the same object.
		c.logger.Debugf("object %s locked, ignoring handling", objectID)
		return nil
	}
	defer c.objectLock.Release(objectID)

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
			c.metricssvc.ObserveControllerHandleLatency(start, metrics.DeleteHandleKind, err == nil)
		}(time.Now())

		return c.handler.Delete(context.Background(), objectID)
	case objectStatusPresent:
		// Get the object data from the storage.
		startStoreGet := time.Now()
		object, err := c.storage.Get(objectID)
		c.metricssvc.ObserveControllerStorageGetLatency(startStoreGet, err == nil)
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
		err = c.handler.Add(context.Background(), object)
		c.metricssvc.ObserveControllerHandleLatency(startHandle, metrics.AddHandleKind, err == nil)
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
		c.metricssvc.IncControllerQueuedTotal()

		// When finishing this func we measure the latency of being consumed a.k.a the
		// time spent on the queue.
		defer func(start time.Time) {
			// If we are here means that the queued object has been consumed form the queue.
			c.metricssvc.ObserveControllerOnQueueLatency(start)
		}(time.Now())

		// Queue the object.
		c.queue <- objectID
	}()
}

// objectLock will be used to lock the objects, this gives us the ability
// to only work with one object at the same time while we can process
// multiple objects in parallel.
// At this moment is a private interface, but could be exposed to the user
// for custom implementations, this way it could be shared the sate between
// different controller instances/replicas.
type objectLock interface {
	Acquire(id string) bool
	Release(id string)
}

type memoryLock struct {
	objectIDs map[string]struct{}
	mu        sync.Mutex
}

func newMemoryLock() objectLock {
	return &memoryLock{
		objectIDs: map[string]struct{}{},
	}
}

func (m *memoryLock) Acquire(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objectIDs[id]
	if ok {
		return false
	}
	m.objectIDs[id] = struct{}{}
	return true
}
func (m *memoryLock) Release(id string) {
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
