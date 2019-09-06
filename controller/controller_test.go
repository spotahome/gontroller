package controller_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/spotahome/gontroller/controller"
	mcontroller "github.com/spotahome/gontroller/internal/mocks/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// callRecorder is a simple concurrency safe called times recorder.
type callRecorder struct {
	i  int
	mu sync.Mutex
}

func (c *callRecorder) inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.i++
}

func (c *callRecorder) times() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.i
}

type testObject struct {
	ID string
}

func TestController(t *testing.T) {
	tests := map[string]struct {
		cfg  controller.Config
		mock func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{})
	}{
		"A regular controller with 1 worker should handle the events from the list correctly.": {
			cfg: controller.Config{
				Workers: 1,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything, mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock getting 100 objects to process.
				objQ := 100
				objs := make([]string, objQ)
				for i := 0; i < objQ; i++ {
					objs[i] = fmt.Sprintf("darknight/batman-%d", i)
				}
				mlw.On("List", mock.Anything, mock.Anything).Once().Return(objs, nil)

				rec := callRecorder{}
				for _, obj := range objs {
					// Expect to Get the object data for each object.
					r := &testObject{
						ID: obj,
					}
					ms.On("Get", mock.Anything, obj).Once().Return(r, nil)

					// Expect to handle added each object with it's data.
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times, then finish.
						rec.inc()
						if rec.times() == objQ {
							close(finishedC)
						}
					})
				}
			},
		},

		"A regular controller with multiple worker should handle the objects from the list correctly only once per object.": {
			cfg: controller.Config{
				Workers: 4,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything, mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock getting 100 objects to process.
				objQ := 100
				objs := make([]string, objQ)
				for i := 0; i < objQ; i++ {
					objs[i] = fmt.Sprintf("darknight/batman-%d", i)
				}
				mlw.On("List", mock.Anything).Once().Return(objs, nil)

				rec := callRecorder{}
				for _, obj := range objs {
					// Expect to Get the object data for each object.
					r := &testObject{
						ID: obj,
					}
					ms.On("Get", mock.Anything, obj).Once().Return(r, nil)

					// Expect to handle added each object with it's data.
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == objQ {
							close(finishedC)
						}
					})
				}
			},
		},

		"A regular controller with multiple worker should handle concurrently streaming object events from different kinds.": {
			cfg: controller.Config{
				Workers: 4,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy lister.
				mlw.On("List", mock.Anything, mock.Anything).Once().Return([]string{}, nil)

				// Mock  10 adds, 10 modifies and 10 deletes object events .
				repQ := 10
				repC := make(chan controller.Event)
				mlw.On("Watch", mock.Anything, mock.Anything).Return((<-chan controller.Event)(repC), nil)
				// Adds.
				go func() {
					time.Sleep(5 * time.Millisecond) // Just in case leave time to the set the mocks.
					for i := 0; i < repQ; i++ {
						repC <- controller.Event{
							ID:   fmt.Sprintf("darknight/batman-add-%d", i),
							Kind: controller.EventAdded,
						}
					}
				}()
				// Modifies.
				go func() {
					time.Sleep(5 * time.Millisecond) // Just in case leave time to the set the mocks.
					for i := 0; i < repQ; i++ {
						repC <- controller.Event{
							ID:   fmt.Sprintf("darknight/batman-mod-%d", i),
							Kind: controller.EventModified,
						}
					}
				}()
				// Deletes.
				go func() {
					time.Sleep(5 * time.Millisecond) // Just in case leave time to the set the mocks.
					for i := 0; i < repQ; i++ {
						repC <- controller.Event{
							ID:   fmt.Sprintf("darknight/batman-del-%d", i),
							Kind: controller.EventDeleted,
						}
					}
				}()

				// The checks of the events.
				maxCalls := 30
				rec := callRecorder{}

				// Adds.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-add-%d", i)
					r := &testObject{ID: id}
					ms.On("Get", mock.Anything, id).Once().Return(r, nil)
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == maxCalls {
							finishedC <- struct{}{}
						}
					})
				}
				// Modifies.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-mod-%d", i)
					r := &testObject{ID: id}
					ms.On("Get", mock.Anything, id).Once().Return(r, nil)
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == maxCalls {
							finishedC <- struct{}{}
						}
					})
				}
				// Deletes.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-del-%d", i)
					mh.On("Delete", mock.Anything, id).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == repQ {
							finishedC <- struct{}{}
						}
					})
				}
			},
		},

		"Multiple object enqueues of the same object should only processed once and with the latest state.": {
			cfg: controller.Config{
				Workers: 1,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// List 1 objects (this will work as an add that will slow the watchers).
				mlw.On("List", mock.Anything, mock.Anything).Once().Return([]string{"slow"}, nil)

				// Guarantee the first event has reached by waiting a bit
				time.After(10 * time.Millisecond)
				// Send the same event lots of times, it should be received only once
				repC := make(chan controller.Event)
				mlw.On("Watch", mock.Anything, mock.Anything).Return((<-chan controller.Event)(repC), nil)
				go func() {
					for i := 0; i < 5; i++ {
						repC <- controller.Event{
							ID:   "single",
							Kind: controller.EventDeleted,
						}
					}
				}()

				// The checks of the events. We should receive one Get for the first object (the second one is a delete)
				rec := callRecorder{}
				r := testObject{ID: "slow"}
				ms.On("Get", mock.Anything, "slow").Once().Return(r, nil).After(30 * time.Millisecond)

				repQ := 2 // Wait both calls.
				r = testObject{ID: "slow"}
				mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
					// If we reach the expected call times then finish.
					rec.inc()
					if rec.times() == repQ {
						finishedC <- struct{}{}
					}
				})

				mh.On("Delete", mock.Anything, "single").Once().Return(nil).Run(func(_ mock.Arguments) {
					// If we reach the expected call times then finish.
					rec.inc()
					if rec.times() == repQ {
						finishedC <- struct{}{}
					}
				})

			},
		},

		"An object that errors and has no retries should only be handled once.": {
			cfg: controller.Config{
				Workers:    1,
				MaxRetries: 0,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything, mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock 2 objects.
				objs := []string{"obj1", "obj2"}
				mlw.On("List", mock.Anything, mock.Anything).Once().Return(objs, nil)

				// The checks of the events.
				rec := callRecorder{}
				for _, id := range objs {
					r := &testObject{ID: id}
					ms.On("Get", mock.Anything, id).Once().Return(r, nil)
					// Return error to check retries.
					mh.On("Add", mock.Anything, r).Once().Return(errors.New("wanted error")).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == len(objs) {
							close(finishedC)
						}
					})
				}
			},
		},

		"An object that errors and has retries should be handled multiple times.": {
			cfg: controller.Config{
				Workers:    1,
				MaxRetries: 5,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything, mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock 2 objects.
				objs := []string{"obj1", "obj2"}
				mlw.On("List", mock.Anything, mock.Anything).Once().Return(objs, nil)

				// The checks of the events.
				rec := callRecorder{}
				retryFactor := 6 // The times should be (retries  + 1).
				for _, id := range objs {
					r := &testObject{ID: id}
					ms.On("Get", mock.Anything, id).Times(retryFactor).Return(r, nil)
					// Return error to check retries.
					mh.On("Add", mock.Anything, r).Times(retryFactor).Return(errors.New("wanted error")).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						rec.inc()
						if rec.times() == len(objs)*retryFactor {
							close(finishedC)
						}

					})
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)

			// Mocks
			mlw := &mcontroller.ListerWatcher{}
			mh := &mcontroller.Handler{}
			ms := &mcontroller.Storage{}
			finishc := make(chan struct{})
			test.mock(mlw, mh, ms, finishc)

			test.cfg.ListerWatcher = mlw
			test.cfg.Handler = mh
			test.cfg.Storage = ms
			ctrl, _ := controller.New(test.cfg)
			ctx, cancel := context.WithCancel(context.Background())
			go ctrl.Run(ctx)
			defer cancel()

			// Wait until finished (if not it will fail the test by timeout) and
			// assert the expectations of the mocks.
			select {
			case <-time.After(2 * time.Second):
				assert.Fail("timeout executing test")
			case <-finishc:
				time.Sleep(100 * time.Millisecond) // Wait a little bit in case there are other goroutines finishing.
				mlw.AssertExpectations(t)
				mh.AssertExpectations(t)
				ms.AssertExpectations(t)
			}
		})
	}
}
