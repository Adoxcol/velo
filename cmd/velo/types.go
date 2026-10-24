package main

// thumbResult describes a visual preview for a download row.
// Mode is "cover" for video/image thumbnails (fills the tile) or "icon" for
// native Windows file/application icons (centered inside the tile).
type thumbResult struct {
	Src  string `json:"src"`
	Mode string `json:"mode"` // "cover" | "icon" | ""
}

type videoToolInfo struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type videoToolsState struct {
	Tools      []videoToolInfo `json:"tools"`
	Installing bool            `json:"installing"`
	Step       string          `json:"step,omitempty"`
	Error      string          `json:"error,omitempty"`
}
