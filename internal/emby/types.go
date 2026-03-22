// Package emby provides programmatic registration of iptvTunerr with Emby and
// Jellyfin media servers. Both servers share the same Live TV API so a single
// implementation covers both. The ServerType field on Config controls log
// prefixes; no other behaviour differs between the two.
package emby

// TunerHostInfo is the JSON body for POST /LiveTv/TunerHosts and its response.
type TunerHostInfo struct {
	Id                  string `json:"Id,omitempty"`
	Type                string `json:"Type"`
	Url                 string `json:"Url"`
	FriendlyName        string `json:"FriendlyName,omitempty"`
	TunerCount          int    `json:"TunerCount,omitempty"`
	ImportFavoritesOnly bool   `json:"ImportFavoritesOnly"`
	AllowHWTranscoding  bool   `json:"AllowHWTranscoding"`
	AllowStreamSharing  bool   `json:"AllowStreamSharing"`
	EnableStreamLooping bool   `json:"EnableStreamLooping"`
	IgnoreDts           bool   `json:"IgnoreDts"`
}

// ListingsProviderInfo is the JSON body for POST /LiveTv/ListingProviders and its response.
type ListingsProviderInfo struct {
	Id              string `json:"Id,omitempty"`
	Type            string `json:"Type"`
	Path            string `json:"Path"`
	EnableAllTuners bool   `json:"EnableAllTuners"`
}

// ScheduledTask is a single item from GET /ScheduledTasks.
type ScheduledTask struct {
	Id                        string  `json:"Id"`
	Key                       string  `json:"Key"`
	Name                      string  `json:"Name"`
	State                     string  `json:"State,omitempty"`
	IsRunning                 bool    `json:"IsRunning,omitempty"`
	CurrentProgressPercentage float64 `json:"CurrentProgressPercentage,omitempty"`
}

// LiveTvChannelList is the response shape of GET /LiveTv/Channels.
// Only TotalRecordCount is used (by the watchdog health check).
type LiveTvChannelList struct {
	TotalRecordCount int `json:"TotalRecordCount"`
}

// LibraryInfo is a simplified view of a configured Emby/Jellyfin library.
type LibraryInfo struct {
	ID             string   `json:"id,omitempty"`
	Name           string   `json:"name"`
	CollectionType string   `json:"collection_type"`
	Locations      []string `json:"locations,omitempty"`
	ItemCount      int      `json:"item_count,omitempty"`
}

// VirtualFolderQueryResult is the response shape of GET /Library/VirtualFolders/Query.
type VirtualFolderQueryResult struct {
	Items            []VirtualFolderInfo `json:"Items"`
	TotalRecordCount int                 `json:"TotalRecordCount"`
}

// VirtualFolderInfo is one configured library/virtual folder.
type VirtualFolderInfo struct {
	Name           string   `json:"Name"`
	CollectionType string   `json:"CollectionType"`
	ItemID         string   `json:"ItemId"`
	ID             string   `json:"Id"`
	Locations      []string `json:"Locations"`
}

// AddVirtualFolder is the request body for POST /Library/VirtualFolders.
type AddVirtualFolder struct {
	Name           string   `json:"Name"`
	CollectionType string   `json:"CollectionType"`
	RefreshLibrary bool     `json:"RefreshLibrary"`
	Paths          []string `json:"Paths"`
}

// ItemQueryResult is the response shape of GET /Items.
type ItemQueryResult struct {
	TotalRecordCount int `json:"TotalRecordCount"`
}

// ItemInfo is a minimal item view returned by GET /Items.
type ItemInfo struct {
	Name     string `json:"Name"`
	SortName string `json:"SortName"`
}

// ItemListResult is the response shape of GET /Items when item rows are needed.
type ItemListResult struct {
	Items            []ItemInfo `json:"Items"`
	TotalRecordCount int        `json:"TotalRecordCount"`
}

// LibraryScanStatus is a simplified best-effort view of the media-server library scan task.
type LibraryScanStatus struct {
	TaskID          string  `json:"task_id,omitempty"`
	TaskKey         string  `json:"task_key,omitempty"`
	TaskName        string  `json:"task_name,omitempty"`
	State           string  `json:"state,omitempty"`
	Running         bool    `json:"running"`
	ProgressPercent float64 `json:"progress_percent,omitempty"`
}
