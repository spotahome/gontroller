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
}

func (c *Config) defaults() {
	if c.ResyncInterval == 0 {
		c.ResyncInterval = 30 * time.Second
	}

	if c.Workers == 0 {
		c.Workers = 1
	}
}

// objectLock will be used to lock the objects, this gives us the ability
// to only work with one object at the same time while we can process
// multiple objects in parallel.
type objectLock struct {
	ids map[string]struct{}
	mu  sync.Mutex
}

func (o *objectLock) acquire(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.ids[id]
	if ok {
		return false
	}
	o.ids[id] = struct{}{}
	return true
}
func (o *objectLock) release(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.ids, id)
}

type objectStatus string

const (
	objectStatusPresent objectStatus = "present"
	objectStatusMissing objectStatus = "missing"
)

// objectStatuses will store the object information in one place so if the
// state of the object changes while is in the queue, it will processed
// with the latest state.
// Example, a object X has been modified enters in the queue, but before it has
// been processed the object has been deleted, we get the object, if
// we don't have an status store we would process the object as being modified
// but the current state of the object is deleted, with this store we only
// would enqueue objectIDs and in the last moment being dequeued the object ID from
// the queue we would check the status on this object to get the latest state.
type objectStatusStore struct {
	objects map[string]objectStatus
	mu      sync.Mutex
}

func (o *objectStatusStore) Store(objectID string, status objectStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.objects[objectID] = status
}

func (o *objectStatusStore) Get(objectID string) (st objectStatus, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st, ok = o.objects[objectID]
	return st, ok
}

func (o *objectStatusStore) Delete(objectID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.objects, objectID)
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
	objectCache objectStatusStore
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
		cfg:        cfg,
		lw:         lw,
		handler:    handler,
		storage:    storage,
		metricssvc: metricssvc.WithID(cfg.Name),
		logger:     logger,
		objectLock: objectLock{
			ids: map[string]struct{}{},
		},
		queue: make(chan string),
		objectCache: objectStatusStore{
			objects: map[string]objectStatus{},
		},
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
		c.enqueueObject(id, objectStatusPresent)
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
				c.enqueueObject(ev.ID, objectStatusPresent)
			case EventDeleted:
				c.enqueueObject(ev.ID, objectStatusMissing)
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
			err := c.processObject(objectID)
			if err != nil {
				// TODO retries.
				c.logger.Errorf("error processing object %s: %s", objectID, err)
			}
		}
	}
}

func (c *Controller) processObject(objectID string) (err error) {
	if !c.objectLock.acquire(objectID) {
		// Could not acquire the object processing, other
		// worker should be processing the same object.
		c.logger.Debugf("object %s locked, ignoring handling", objectID)
		return nil
	}
	defer c.objectLock.release(objectID)

	// What's the status of the object? We allow enqueuing when finished.
	startStoreGet := time.Now()
	status, ok := c.objectCache.Get(objectID)
	c.metricssvc.ObserveControllerStoreGetLatency(startStoreGet, ok)
	if !ok {
		c.logger.Errorf("%s object status not present in cache", objectID)
		return nil
	}

	defer c.objectCache.Delete(objectID)

	// Observe metrics of handling the object.
	defer func(start time.Time) {
		c.metricssvc.ObserveControllerHandleLatency(start, string(status), err == nil)
	}(time.Now())

	// Call the handler logic.
	switch status {
	case objectStatusMissing:
		return c.handler.Delete(context.Background(), objectID)
	case objectStatusPresent:
		object, err := c.storage.Get(objectID)
		if err != nil {
			return err
		}
		if object == nil {
			c.logger.Debugf("object %s has been ignored by the store", objectID)
			return nil
		}
		return c.handler.Add(context.Background(), object)
	default:
		return fmt.Errorf("unknown object status kind: %s", status)
	}
}

func (c *Controller) enqueueObject(objectID string, status objectStatus) {
	// Is already queued our event? If the event is already queued
	// then don't enqueue again, we only need to update the status/
	// on the cache.
	_, ok := c.objectCache.Get(objectID)

	// We need to store the state on cache before enqueuing, if we
	// don't do this, the workers could get a new object to handle
	// and they don't have the latest object information on the cache.
	c.objectCache.Store(objectID, status)

	// Enqueue only if necessary. This way we don't enqueue the same
	// object to process multiple times.
	if !ok {
		go func() {
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
}
