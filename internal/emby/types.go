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
	Id   string `json:"Id"`
	Key  string `json:"Key"`
	Name string `json:"Name"`
}

// LiveTvChannelList is the response shape of GET /LiveTv/Channels.
// Only TotalRecordCount is used (by the watchdog health check).
type LiveTvChannelList struct {
	TotalRecordCount int `json:"TotalRecordCount"`
}
