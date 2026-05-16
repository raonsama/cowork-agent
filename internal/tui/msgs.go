// Package tui — message types for the Bubble Tea update loop.
package tui

import (
	"time"

	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/thermal"
)

type (
	agentEventMsg agent.Event    // relayed from agent.Events()
	thermalMsg    thermal.Status // polled on every tick
	tickMsg       time.Time      // drives periodic background work
	fileListMsg   []string       // result of async project walk
)
