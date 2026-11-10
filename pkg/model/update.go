package model

// UpgradeResult represents the outcome of a self-update check or installation.
type UpgradeResult struct {
	CurrentVersion   string `json:"current_version"`
	TargetVersion    string `json:"target_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Applied          bool   `json:"applied"`
	ReleaseName      string `json:"release_name,omitempty"`
	ReleaseURL       string `json:"release_url,omitempty"`
	PublishedAt      string `json:"published_at,omitempty"`
	AssetsTotal      int64  `json:"assets_total_bytes,omitempty"`
	AssetName        string `json:"asset_name,omitempty"`
	AssetSize        int64  `json:"asset_size,omitempty"`
	Checksum         string `json:"checksum,omitempty"`
	ChecksumVerified bool   `json:"checksum_verified"`
	TargetPath       string `json:"target_path,omitempty"`
	Message          string `json:"message"`
}
