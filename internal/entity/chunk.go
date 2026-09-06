package entity

const (
	SectorSize      = 2048
	BlockSize       = 128 << 10
	BootBytes       = 64 << 20
	ManifestVersion = 2
)

type Manifest struct {
	Version         int        `json:"version"`
	Serial          string     `json:"serial"`
	RedumpHash      string     `json:"redump_hash"`
	Title           string     `json:"title"`
	SizeBytes       int64      `json:"size_bytes"`
	BlockSize       int        `json:"block_size"`
	BootRanges      [][2]int64 `json:"boot_ranges"`
	FileURL         string     `json:"file_url"`
	ManifestURL     string     `json:"manifest_url"`
	Supported       bool       `json:"supported"`
	UnsupportedHint string     `json:"unsupported_hint,omitempty"`
}
