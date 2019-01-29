package metrics

import "time"

// Dummy is a dummy metrics service.
var Dummy = &dummy{}

type dummy struct{}

func (d *dummy) WithID(_ string) Recorder                                      { return d }
func (dummy) IncControllerQueuedTotal()                                        {}
func (dummy) ObserveControllerOnQueueLatency(_ time.Time)                      {}
func (dummy) ObserveControllerStorageGetLatency(_ time.Time, _ bool)           {}
func (dummy) ObserveControllerListLatency(_ time.Time, _ bool)                 {}
func (dummy) ObserveControllerHandleLatency(_ time.Time, _ HandleKind, _ bool) {}
