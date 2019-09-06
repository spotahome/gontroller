package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spotahome/gontroller/controller"
	"github.com/spotahome/gontroller/log"
)

// sharedLocker will be shared between all the
type sharedLocker struct {
	objectIDs map[string]struct{}
	mu        sync.Mutex
}

func (s *sharedLocker) Acquire(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objectIDs[id]
	if ok {
		return false
	}
	s.objectIDs[id] = struct{}{}
	return true
}
func (s *sharedLocker) Release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objectIDs, id)
}

func newController(name string, locker controller.ObjectLocker) (*controller.Controller, error) {
	logger := &log.Std{}
	return controller.New(controller.Config{
		Name:           name,
		Logger:         logger,
		ResyncInterval: 5 * time.Second,
		Workers:        2,
		ObjectLocker:   locker,
		ListerWatcher: controller.ListerWatcherFunc{
			ListFunc:  func(_ context.Context) ([]string, error) { return []string{"id1", "id2", "id3", "id4"}, nil },
			WatchFunc: func(_ context.Context) (<-chan controller.Event, error) { return nil, nil },
		},
		Handler: controller.HandlerFunc{
			AddFunc: func(_ context.Context, obj interface{}) error {
				<-time.After(4 * time.Second)
				logger.Infof("controller %s handled %s", name, obj.(string))
				return nil
			},
			DeleteFunc: func(_ context.Context, id string) error { return nil },
		},
		Storage: controller.StorageFunc(func(_ context.Context, id string) (interface{}, error) { return id, nil }),
	})
}

func main() {

	ctx, cancel := context.WithCancel(context.Background())

	locker := &sharedLocker{objectIDs: map[string]struct{}{}}
	for i := 0; i < 10; i++ {
		ctrl, _ := newController(fmt.Sprintf("controller%d", i), locker)
		go func() {
			err := ctrl.Run(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error while running controller: %s", err)
				os.Exit(1)
			}
		}()
	}

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM, syscall.SIGINT)
	<-sigC
	fmt.Println("signal captured, exiting...")
	cancel()
	<-time.After(2 * time.Second)
}
