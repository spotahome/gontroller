package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spotahome/gontroller/controller"
	"github.com/spotahome/gontroller/log"
	"github.com/spotahome/gontroller/metrics"
)

const (
	metricsListenAddr = ":8081"
)

type fakeObject struct {
	ID string
	TS time.Time
}

func main() {
	logger := log.Std{Debug: true}

	// Create prometheus metrics and serve the metrics.
	promreg := prometheus.NewRegistry()
	go func() {
		logger.Infof("serving metrics on %s", metricsListenAddr)
		http.ListenAndServe(metricsListenAddr, promhttp.HandlerFor(promreg, promhttp.HandlerOpts{}))
	}()

	// Create all required components for the controller.
	lw := createListeWatcher()
	st := createStorage()
	h := createHandler(logger)
	metricsrecorder := metrics.NewPrometheus(promreg)

	// Create and run the controller.
	ctrl, err := controller.New(controller.Config{
		Name:            "stub-controller",
		Workers:         3,
		MaxRetries:      2,
		ListerWatcher:   lw,
		Handler:         h,
		Storage:         st,
		MetricsRecorder: metricsrecorder,
		Logger:          logger,
	})
	if err != nil {
		logger.Errorf("error creating controller: %s", err)
		os.Exit(1)
	}

	go func() {
		err = ctrl.Run(context.Background())
		if err != nil {
			logger.Errorf("error running controller: %s", err)
			os.Exit(1)
		}
	}()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM, syscall.SIGINT)
	<-sigC
	logger.Infof("signal captured, exiting...")
}

func createListeWatcher() controller.ListerWatcher {
	return &controller.ListerWatcherFunc{
		ListFunc: func(_ context.Context) ([]string, error) {
			randomSleep()
			return []string{
				"faked-obj1",
				"faked-obj2",
				"faked-obj3",
				"faked-obj4",
				"faked-obj5",
			}, nil
		},
		WatchFunc: func(_ context.Context) (<-chan controller.Event, error) {
			c := make(chan controller.Event)
			go func() {
				t := time.NewTicker(10 * time.Millisecond)
				for range t.C {
					c <- controller.Event{
						ID:   "faked-obj1",
						Kind: controller.EventAdded,
					}
				}
			}()
			return c, nil
		},
	}
}

func createStorage() controller.Storage {
	return controller.StorageFunc(func(_ context.Context, id string) (interface{}, error) {
		randomSleep()
		switch id {
		// 2nd object fail.
		case "faked-obj2":
			return nil, errors.New("error retrieving object")
		// 5th object ignored.
		case "faked-obj5":
			return nil, nil
		// everything else ok:
		default:
			return &fakeObject{
				ID: id,
				TS: time.Now(),
			}, nil
		}
	})
}

func createHandler(logger log.Logger) controller.Handler {
	return &controller.HandlerFunc{
		AddFunc: func(_ context.Context, obj interface{}) error {
			randomSleep()
			fobj := obj.(*fakeObject)
			switch fobj.ID {
			// Fail on 1st object.
			case "faked-obj1":
				time.Sleep(100 * time.Second)
				return errors.New("failed handling object")
			default:
				logger.Infof("added object %s from %s", fobj.ID, fobj.TS)
				return nil
			}
		},
		DeleteFunc: func(_ context.Context, id string) error {
			randomSleep()
			logger.Infof("deleted object %s", id)
			return nil
		},
	}
}

func randomSleep() {
	rand := (time.Now().Second() % 10) * 100
	time.Sleep(time.Duration(rand) * time.Millisecond)
}
