package controller_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/spotahome/gontroller/controller"
	mcontroller "github.com/spotahome/gontroller/internal/mocks/controller"
	"github.com/spotahome/gontroller/log"
	"github.com/spotahome/gontroller/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testObject struct {
	ID string
}

func TestController(t *testing.T) {
	tests := []struct {
		name string
		cfg  controller.Config
		mock func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{})
	}{
		{
			name: "A regular controller with 1 worker should handle the events from the list correctly.",
			cfg: controller.Config{
				Workers: 1,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock getting 100 objects to process.
				objQ := 100
				objs := make([]string, objQ)
				for i := 0; i < objQ; i++ {
					objs[i] = fmt.Sprintf("darknight/batman-%d", i)
				}
				mlw.On("List", mock.Anything).Once().Return(objs, nil)

				timesCalled := 0
				for _, obj := range objs {
					// Expect to Get the object data for each object.
					r := &testObject{
						ID: obj,
					}
					ms.On("Get", obj).Once().Return(r, nil)

					// Expect to handle added each object with it's data.
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times, then finish.
						timesCalled++
						if timesCalled == objQ {
							close(finishedC)
						}
					})
				}
			},
		},
		{
			name: "A regular controller with multiple worker should handle the objects from the list correctly only once per object.",
			cfg: controller.Config{
				Workers: 4,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock getting 100 objects to process.
				objQ := 100
				objs := make([]string, objQ)
				for i := 0; i < objQ; i++ {
					objs[i] = fmt.Sprintf("darknight/batman-%d", i)
				}
				mlw.On("List", mock.Anything).Once().Return(objs, nil)

				timesCalled := 0
				for _, obj := range objs {
					// Expect to Get the object data for each object.
					r := &testObject{
						ID: obj,
					}
					ms.On("Get", obj).Once().Return(r, nil)

					// Expect to handle added each object with it's data.
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == objQ {
							close(finishedC)
						}
					})
				}
			},
		},
		{
			name: "A regular controller with multiple worker should handle concurrently streaming object events from different kinds.",
			cfg: controller.Config{
				Workers: 4,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy lister.
				mlw.On("List", mock.Anything).Once().Return([]string{}, nil)

				// Mock  10 adds, 10 modifies and 10 deletes object events .
				repQ := 10
				repC := make(chan controller.Event)
				mlw.On("Watch", mock.Anything).Return((<-chan controller.Event)(repC), nil)
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
				timesCalled := 0

				// Adds.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-add-%d", i)
					r := &testObject{ID: id}
					ms.On("Get", id).Once().Return(r, nil)
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == maxCalls {
							close(finishedC)
						}
					})
				}
				// Modifies.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-mod-%d", i)
					r := &testObject{ID: id}
					ms.On("Get", id).Once().Return(r, nil)
					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == maxCalls {
							close(finishedC)
						}
					})
				}
				// Deletes.
				for i := 0; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-del-%d", i)
					mh.On("Delete", mock.Anything, id).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == maxCalls {
							close(finishedC)
						}
					})
				}
			},
		},
		{
			name: "Multiple object enqueues of the same object should only processed once and with the latest state.",
			cfg: controller.Config{
				Workers: 1,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// Mock dummy list.
				mlw.On("List", mock.Anything).Once().Return([]string{}, nil)

				// First we will receive 10 add events on the channel, afterwards we will receive 5
				// deletes fo the last 5 object(`darknight/batman-[5-9]``).
				//
				// This will test we only receive 10 events instead of 5 (duplicated keys only processed once)
				// and the last 5 object are processed with the latest state, not the first one
				repC := make(chan controller.Event)
				mlw.On("Watch", mock.Anything).Return((<-chan controller.Event)(repC), nil)
				repQ := 10
				go func() {
					for i := 0; i < repQ; i++ {
						repC <- controller.Event{
							ID:   fmt.Sprintf("darknight/batman-%d", i),
							Kind: controller.EventAdded,
						}
					}
				}()
				go func() {
					time.Sleep(1 * time.Millisecond)
					for i := 5; i < repQ; i++ {
						repC <- controller.Event{
							ID:   fmt.Sprintf("darknight/batman-%d", i),
							Kind: controller.EventDeleted,
						}
					}
				}()

				// The checks of the events.
				// We should have 5 adds and 5 deletes, because the delete events after the first adds
				// should have been update in the internal cache and only process ones.
				timesCalled := 0

				for i := 0; i < 5; i++ {
					id := fmt.Sprintf("darknight/batman-%d", i)
					r := &testObject{ID: id}
					ms.On("Get", id).Once().Return(r, nil).Run(func(_ mock.Arguments) {
						// We need to slow down the handlers to give it time to the streamed
						// events to reach the de enqueue process.
						time.Sleep(1 * time.Millisecond)
					})

					mh.On("Add", mock.Anything, r).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == repQ {
							close(finishedC)
						}
					})
				}

				// Deletes.
				for i := 5; i < repQ; i++ {
					id := fmt.Sprintf("darknight/batman-%d", i)
					mh.On("Delete", mock.Anything, id).Once().Return(nil).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == repQ {
							close(finishedC)
						}
					})
				}
			},
		},
		{
			name: "An object that errors and has no retries should only be handled once.",
			cfg: controller.Config{
				Workers:    1,
				MaxRetries: 0,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock 2 objects.
				objs := []string{"obj1", "obj2"}
				mlw.On("List", mock.Anything).Once().Return(objs, nil)

				// The checks of the events.
				timesCalled := 0
				for _, id := range objs {
					r := &testObject{ID: id}
					ms.On("Get", id).Once().Return(r, nil)
					// Return error to check retries.
					mh.On("Add", mock.Anything, r).Once().Return(errors.New("wanted error")).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == len(objs) {
							close(finishedC)
						}
					})
				}
			},
		},
		{
			name: "An object that errors and has retries should be handled multiple times.",
			cfg: controller.Config{
				Workers:    1,
				MaxRetries: 5,
			},
			mock: func(mlw *mcontroller.ListerWatcher, mh *mcontroller.Handler, ms *mcontroller.Storage, finishedC chan struct{}) {
				// A dummy watcher.
				mlw.On("Watch", mock.Anything).Return(make(<-chan controller.Event), nil)

				// Mock 2 objects.
				objs := []string{"obj1", "obj2"}
				mlw.On("List", mock.Anything).Once().Return(objs, nil)

				// The checks of the events.
				timesCalled := 0
				retryFactor := 6 // The times should be (retries  + 1).
				for _, id := range objs {
					r := &testObject{ID: id}
					ms.On("Get", id).Times(retryFactor).Return(r, nil)
					// Return error to check retries.
					mh.On("Add", mock.Anything, r).Times(retryFactor).Return(errors.New("wanted error")).Run(func(_ mock.Arguments) {
						// If we reach the expected call times then finish.
						timesCalled++
						if timesCalled == len(objs)*retryFactor {
							close(finishedC)
						}
					})
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			// Mocks
			mlw := &mcontroller.ListerWatcher{}
			mh := &mcontroller.Handler{}
			ms := &mcontroller.Storage{}
			finishc := make(chan struct{})
			test.mock(mlw, mh, ms, finishc)

			ctrl := controller.New(test.cfg, mlw, mh, ms, metrics.Dummy, log.Dummy)
			stopC := make(chan struct{})
			go ctrl.Run(stopC)

			// Wait until finished (if not it will fail the test by timeout) and
			// assert the expectations of the mocks.
			select {
			case <-time.After(1 * time.Second):
				assert.Fail("timeout executing test")
			case <-finishc:
				mlw.AssertExpectations(t)
				mh.AssertExpectations(t)
				ms.AssertExpectations(t)
			}
		})
	}
}
