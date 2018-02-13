package loggers

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// RateLogger logs messages but also accumulates the rate at which the
// Log function is being called and adds that to the fields that are printed.
// This is concurrent safe but may not make sense. Since it is storing global
// state on the number of messages received, multiple goroutines interacting
// with it trying to log different things will cause the rates to be incorrect
type RateLogger struct {
	// Stores the number of messages received up until countToPrint
	count float64 // Because easy division. Sue me

	// Time the interval started
	startTime time.Time

	// Total number of messages to accumulate before calculating the rate
	countToPrint float64 // Different reasons. Sue me again
}

// NewRateLogger reates a new RateLogger given the countToPrint argument
func NewRateLogger(countToPrint int) *RateLogger {
	return &RateLogger{
		count:        0.0,
		countToPrint: float64(countToPrint),
		startTime:    time.Now(),
	}
}

// Log logs the given msg and fields to Info. Calculates the rate at which this
// function is being called using the global count and startTime variables
// and adds the rate to the fields being logged
func (r *RateLogger) Log(msg string, fields log.Fields) {
	r.count++
	if r.count >= r.countToPrint {
		endTime := time.Now()
		secs := endTime.Sub(r.startTime).Seconds()
		hz := r.count / secs

		if fields == nil {
			fields = log.Fields{
				"rate": hz,
			}
		} else {
			fields["rate"] = hz
		}

		log.WithFields(fields).Info(msg)
		r.count = 0
		r.startTime = endTime
	}
}

// InitStartTime intializes the start time used to calcuate rates. This should
// only be called once. This function should be used if you intend to
// calculate rates much later than when the logger was constructed
func (r *RateLogger) InitStartTime() {
	r.startTime = time.Now()
}
