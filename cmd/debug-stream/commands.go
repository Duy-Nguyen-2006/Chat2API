package main

import "os"

func dispatchCommand() bool {
	if len(os.Args) <= 1 {
		return false
	}
	switch os.Args[1] {
	case "apps":
		dumpApps()
	case "probe":
		mainProbe()
	case "gizmo":
		os.Args = append(os.Args, "gizmo-only")
		mainProbe()
	case "gizmo-keys":
		mainGizmoKeys()
	case "scan":
		mainScan()
	case "fetch-gizmo":
		mainFetchGizmo()
	case "fetch-conversations":
		mainFetchConversations()
	case "workspaces":
		mainFetchWorkspaces()
	case "gpts":
		mainFetchGPTs()
	case "say":
		mainSay()
	default:
		return false
	}
	return true
}