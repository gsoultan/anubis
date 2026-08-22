package main

import (
	"time"

	"github.com/gsoultan/anubis/internal/repository"
)

// systemClock is the production repository.Clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

var _ repository.Clock = systemClock{}
