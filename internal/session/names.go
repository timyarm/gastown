// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"
)

// Prefix is the common prefix for all Gas Town tmux session names.
const Prefix = "gt-"

// MayorSessionName returns the session name for the Mayor agent.
func MayorSessionName() string {
	return Prefix + "mayor"
}

// DeaconSessionName returns the session name for the Deacon agent.
func DeaconSessionName() string {
	return Prefix + "deacon"
}

// WitnessSessionName returns the session name for a rig's Witness agent.
func WitnessSessionName(rig string) string {
	return fmt.Sprintf("%s%s-witness", Prefix, rig)
}

// RefinerySessionName returns the session name for a rig's Refinery agent.
func RefinerySessionName(rig string) string {
	return fmt.Sprintf("%s%s-refinery", Prefix, rig)
}

// CrewSessionName returns the session name for a crew worker in a rig.
func CrewSessionName(rig, name string) string {
	return fmt.Sprintf("%s%s-crew-%s", Prefix, rig, name)
}

// PolecatSessionName returns the session name for a polecat in a rig.
func PolecatSessionName(rig, name string) string {
	return fmt.Sprintf("%s%s-%s", Prefix, rig, name)
}

// SessionBeacon generates an identity beacon message for Claude Code sessions.
// This beacon becomes the session title in /resume picker, enabling workers to
// find their predecessor sessions.
//
// Format: [GAS TOWN] <address> • <mol-id or "ready"> • <timestamp>
//
// Examples:
//   - [GAS TOWN] gastown/crew/max • gt-abc12 • 2025-12-30T14:32
//   - [GAS TOWN] gastown/polecats/Toast • ready • 2025-12-30T09:15
//   - [GAS TOWN] deacon • patrol • 2025-12-30T08:00
func SessionBeacon(address, molID string) string {
	if molID == "" {
		molID = "ready"
	}
	// Use local time in a compact format
	timestamp := time.Now().Format("2006-01-02T15:04")
	return fmt.Sprintf("[GAS TOWN] %s • %s • %s", address, molID, timestamp)
}

// PropulsionNudge generates the GUPP (Gas Town Universal Propulsion Principle) nudge.
// This is sent after the beacon to trigger autonomous work execution.
// The agent receives this as user input, triggering the propulsion principle:
// "If work is on your hook, YOU RUN IT."
func PropulsionNudge() string {
	return "Run `gt mol status` to check your hook and begin work."
}
