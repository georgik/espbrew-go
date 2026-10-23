package devicesleep

import "errors"

var (
	// ErrUnknownDevice is returned when a board that was never registered is
	// asked to wake, sleep, or be touched.
	ErrUnknownDevice = errors.New("device not registered in sleep manager")

	// ErrBusy is returned when a board that is mid-operation is asked to
	// power-cycle. Cutting its power mid-flash would corrupt the transfer.
	ErrBusy = errors.New("device is busy - power state unchanged")
)
