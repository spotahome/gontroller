package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spotahome/gontroller/controller"
	"github.com/spotahome/gontroller/log"
)

type superHero struct {
	ID      string
	Name    string
	Company string
}

var database = map[string]superHero{
	"Batman":    superHero{ID: "Batman", Name: "Bruce Wayne", Company: "DC"},
	"Spiderman": superHero{ID: "Spiderman", Name: "Peter Parker", Company: "Marvel"},
	"Deadpool":  superHero{ID: "Deadpool", Name: "Wade Wilson", Company: "Marvel"},
}

func main() {
	logger := &log.Std{}

	ctrl, err := controller.New(controller.Config{
		Logger:         logger,
		ResyncInterval: 5 * time.Second,
		Workers:        2,
		MaxRetries:     2,
		ListerWatcher: controller.ListerWatcherFunc{
			ListFunc: func(_ context.Context) ([]string, error) {
				askFor := []string{}
				for id := range database {
					askFor = append(askFor, id)
				}
				return askFor, nil
			},
			WatchFunc: func(_ context.Context) (<-chan controller.Event, error) {
				c := make(chan controller.Event)
				go func() {
					<-time.After(15 * time.Second)
					database["Wolverine"] = superHero{ID: "Wolverine", Name: "James (Logan) Howlett", Company: "Marvel"}
					c <- controller.Event{ID: "Wolverine", Kind: controller.EventAdded}

					<-time.After(15 * time.Second)
					delete(database, "Spiderman")
					c <- controller.Event{ID: "Spiderman", Kind: controller.EventDeleted}
				}()
				return c, nil
			},
		},
		Handler: controller.HandlerFunc{
			AddFunc: func(_ context.Context, obj interface{}) error {
				sh, ok := obj.(superHero)
				if !ok {
					return nil
				}
				if time.Now().Nanosecond()%2 == 0 {
					return fmt.Errorf("could not process superhero '%s'", sh.ID)
				}

				logger.Infof("%s from %s is %s", sh.ID, sh.Company, sh.Name)
				<-time.After(3 * time.Second)

				return nil
			},
			DeleteFunc: func(_ context.Context, id string) error {
				logger.Infof("%s deleted", id)
				return nil
			},
		},
		Storage: controller.StorageFunc(func(_ context.Context, id string) (interface{}, error) {
			obj, ok := database[id]
			if !ok {
				return nil, nil
			}

			return obj, nil
		}),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating controller: %s", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := ctrl.Run(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while running controller: %s", err)
			os.Exit(1)
		}
	}()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM, syscall.SIGINT)
	<-sigC
	logger.Infof("signal captured, exiting...")
	cancel()
	<-time.After(2 * time.Second)
}
