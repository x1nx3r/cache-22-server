package entity

import (
	"testing"
)

func TestRangeConstants(t *testing.T) {
	if SectorSize != 2048 {
		t.Errorf("SectorSize = %d, want 2048", SectorSize)
	}
	if BlockSize != 128<<10 {
		t.Errorf("BlockSize = %d, want 131072", BlockSize)
	}
	if BlockSize%SectorSize != 0 {
		t.Error("BlockSize must stay a whole number of sectors")
	}
	if BootBytes != 64<<20 {
		t.Errorf("BootBytes = %d, want 64MB", BootBytes)
	}
	if ManifestVersion != 2 {
		t.Errorf("ManifestVersion = %d, want 2", ManifestVersion)
	}
}
