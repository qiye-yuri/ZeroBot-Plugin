package updatewatch

import "time"

// Admin is implemented by the running plugin so UI and scheduler share a lock.
type Admin func(revision string, config *Config) (AdminSnapshot, error)

// RuntimeAdmin is set once by plugin initialization before main starts the UI.
// Keep the plugin's blank import: the priority registry scans blank imports.
var RuntimeAdmin Admin

type ModelOption struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}
type AdminSnapshot struct {
	Config       Config            `json:"config"`
	Revision     string            `json:"revision"`
	Owners       []int64           `json:"owners"`
	Bots         []int64           `json:"bots"`
	Models       []ModelOption     `json:"models"`
	LastCheck    time.Time         `json:"lastCheck"`
	Pending      int               `json:"pending"`
	Baseline     int               `json:"baseline"`
	LastError    string            `json:"lastError"`
	SourceErrors map[string]string `json:"sourceErrors"`
}
