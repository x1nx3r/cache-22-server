package web

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func bootBytes(m entity.Manifest) int64 {
	var n int64
	for _, r := range m.BootRanges {
		n += r[1] - r[0]
	}
	return n
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func coverURL(serial string) string {
	return "/v1/games/" + url.PathEscape(serial) + "/cover"
}
