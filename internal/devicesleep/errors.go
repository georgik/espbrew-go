package devicesleep

import "errors"

var (
	// ErrUnknownDevice is returned when a board that was never registered is
	// asked to wake, sleep, or be touched.
	ErrUnknownDevice = errors.New("device not registered in sleep manager")
)
