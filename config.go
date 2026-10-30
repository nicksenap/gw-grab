package main

import (
	"os"

	"github.com/nicksenap/gw-grab/internal/dispatch"
)

// agentCmd is the agent started for a dispatch when the panel doesn't send one
// (GW_GRAB_AGENT_CMD, default pi with the pocock skill profile).
//
// There are no API tokens here: page context arrives from the browser extension
// (which is already authenticated), so the host never talks to those services.
func agentCmd() string {
	if v := os.Getenv("GW_GRAB_AGENT_CMD"); v != "" {
		return v
	}
	return dispatch.DefaultAgentCmd
}
