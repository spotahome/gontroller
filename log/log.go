package log

import (
	"fmt"
	"log"
)

// Logger knows how to log messages in the go fmt style.
type Logger interface {
	Infof(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// Dummy is a dummy logger that doesn't log.
var Dummy = &dummy{}

type dummy struct{}

func (dummy) Infof(format string, args ...interface{})    {}
func (dummy) Warningf(format string, args ...interface{}) {}
func (dummy) Errorf(format string, args ...interface{})   {}
func (dummy) Debugf(format string, args ...interface{})   {}

// Std satisfies Logger interface using the standard go logger.
type Std struct {
	Debug bool
}

// Infof satisfies Logger interface.
func (s Std) Infof(format string, args ...interface{}) {
	log.Printf(fmt.Sprintf("[INFO] %s \n", format), args...)
}

// Warningf satisfies Logger interface.
func (s Std) Warningf(format string, args ...interface{}) {
	log.Printf(fmt.Sprintf("[WARN] %s \n", format), args...)
}

// Errorf satisfies Logger interface.
func (s Std) Errorf(format string, args ...interface{}) {
	log.Printf(fmt.Sprintf("[ERROR] %s \n", format), args...)
}

// Debugf satisfies Logger interface.
func (s Std) Debugf(format string, args ...interface{}) {
	if s.Debug {
		log.Printf(fmt.Sprintf("[DEBUG] %s \n", format), args...)
	}
}
