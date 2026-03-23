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

// LiveTVInfo is a coarse status view returned by GET /LiveTv/Info.
type LiveTVInfo struct {
	IsEnabled bool            `json:"IsEnabled"`
	Services  []LiveTVService `json:"Services"`
}

// LiveTVService is one service row from GET /LiveTv/Info.
type LiveTVService struct {
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

// LiveTVConfiguration is the response shape of GET /System/Configuration/livetv.
type LiveTVConfiguration struct {
	TunerHosts       []TunerHostInfo        `json:"TunerHosts"`
	ListingProviders []ListingsProviderInfo `json:"ListingProviders"`
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

// UserInfo is a simplified Emby/Jellyfin user record.
type UserInfo struct {
	ID                     string         `json:"Id,omitempty"`
	Name                   string         `json:"Name"`
	ServerID               string         `json:"ServerId,omitempty"`
	Policy                 map[string]any `json:"Policy,omitempty"`
	HasPassword            bool           `json:"HasPassword,omitempty"`
	HasConfiguredPassword  bool           `json:"HasConfiguredPassword,omitempty"`
	HasConfiguredEasyPwd   bool           `json:"HasConfiguredEasyPassword,omitempty"`
	EnableAutoLogin        bool           `json:"EnableAutoLogin,omitempty"`
	EnableLocalPassword    bool           `json:"EnableLocalPassword,omitempty"`
	EnableRemoteAccess     bool           `json:"EnableRemoteAccess,omitempty"`
	EnableContentDeletion  bool           `json:"EnableContentDeletion,omitempty"`
	IsAdministrator        bool           `json:"IsAdministrator,omitempty"`
	IsHidden               bool           `json:"IsHidden,omitempty"`
	IsDisabled             bool           `json:"IsDisabled,omitempty"`
	InvalidLoginAttemptCnt int            `json:"InvalidLoginAttemptCount,omitempty"`
}

// DesiredUserPolicy is the additive subset of user policy that Tunerr can
// safely infer from Plex identity/share state without guessing folder-level or
// SSO-only permissions.
type DesiredUserPolicy struct {
	EnableLiveTvAccess       *bool `json:"enable_live_tv_access,omitempty"`
	EnableRemoteAccess       *bool `json:"enable_remote_access,omitempty"`
	EnableContentDownloading *bool `json:"enable_content_downloading,omitempty"`
	EnableSyncTranscoding    *bool `json:"enable_sync_transcoding,omitempty"`
	EnableAllFolders         *bool `json:"enable_all_folders,omitempty"`
}
