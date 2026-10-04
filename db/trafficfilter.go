package db

import (
	"time"

	"gorm.io/gorm"
)

// TrafficFilter narrows the traffic_metrics rows that every statistics query
// (summary counts, group-by-counts, request details, time series) looks at.
// Zero-valued fields are not applied.
type TrafficFilter struct {
	Start *time.Time
	End   *time.Time
	// IsStatic selects only static-asset requests (true) or only non-static ones (false).
	IsStatic        *bool
	StatusCode      *int
	Country         string
	DeviceType      string
	Platform        string
	Browser         string
	Fingerprint     string
	FingerprintType string
	// Username selects one user's requests; GuestUsername selects unauthenticated ones.
	Username string
}

// GuestUsername is the label CountBy(DimensionUser) gives unauthenticated traffic.
const GuestUsername = "guest"

// Apply adds the filter's conditions to q, which must be a query on
// traffic_metrics. Bounds are converted to UTC: timestamps are stored as UTC
// text and compared as text, so a non-UTC bound would match the wrong rows.
func (f TrafficFilter) Apply(q *gorm.DB) *gorm.DB {
	if f.Start != nil {
		q = q.Where("traffic_metrics.timestamp >= ?", f.Start.UTC())
	}
	if f.End != nil {
		q = q.Where("traffic_metrics.timestamp <= ?", f.End.UTC())
	}
	if f.IsStatic != nil {
		q = q.Where("traffic_metrics.is_static_asset = ?", *f.IsStatic)
	}
	if f.StatusCode != nil {
		q = q.Where("traffic_metrics.http_status = ?", *f.StatusCode)
	}
	for _, c := range []struct{ column, value string }{
		{"country", f.Country},
		{"device_family", f.DeviceType},
		{"os_family", f.Platform},
		{"browser_family", f.Browser},
		{"fingerprint", f.Fingerprint},
		{"fingerprint_type", f.FingerprintType},
	} {
		if c.value != "" {
			q = q.Where("traffic_metrics."+c.column+" = ?", c.value)
		}
	}
	switch f.Username {
	case "":
	case GuestUsername:
		q = q.Where("(traffic_metrics.user_id = '' OR traffic_metrics.user_id IS NULL)")
	default:
		q = q.Where("traffic_metrics.user_id IN (SELECT id FROM users WHERE username = ?)", f.Username)
	}
	return q
}

// Dimension is a column (or expression) CountBy can group requests by.
type Dimension string

const (
	DimensionStatus          Dimension = "status"
	DimensionCountry         Dimension = "country"
	DimensionDeviceType      Dimension = "device_type"
	DimensionPlatform        Dimension = "platform"
	DimensionBrowser         Dimension = "browser"
	DimensionUser            Dimension = "user"
	DimensionFingerprint     Dimension = "fingerprint"
	DimensionFingerprintType Dimension = "fingerprint_type"
)

type dimensionSpec struct {
	expr string
	join string
}

// dimensionSpecs is an allow-list: expr and join are spliced into SQL.
var dimensionSpecs = map[Dimension]dimensionSpec{
	DimensionStatus:          {expr: "traffic_metrics.http_status"},
	DimensionCountry:         {expr: "traffic_metrics.country"},
	DimensionDeviceType:      {expr: "traffic_metrics.device_family"},
	DimensionPlatform:        {expr: "traffic_metrics.os_family"},
	DimensionBrowser:         {expr: "traffic_metrics.browser_family"},
	DimensionFingerprint:     {expr: "traffic_metrics.fingerprint"},
	DimensionFingerprintType: {expr: "traffic_metrics.fingerprint_type"},
	DimensionUser: {
		expr: "COALESCE(users.username, '" + GuestUsername + "')",
		join: "LEFT JOIN users ON users.id = traffic_metrics.user_id",
	},
}
