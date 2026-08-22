package main

import (
	"time"

	"github.com/gsoultan/anubis/internal/shared/clock"
)

// systemClock is the production clock.Clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

var _ clock.Clock = systemClock{}
