package handlers

import (
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
)

// trafficFilterParams is the set of query parameters every statistics
// endpoint shares (api components/parameters/Traffic*). The summary and
// details endpoints' generated Params structs convert to it directly.
type trafficFilterParams api.GetRequestStatisticsParams

func (p trafficFilterParams) filter() db.TrafficFilter {
	f := db.TrafficFilter{
		Start:      p.StartDate,
		End:        p.EndDate,
		IsStatic:   p.IsStatic,
		StatusCode: p.StatusCode,
	}
	setIfPresent := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	setIfPresent(&f.Country, p.Country)
	setIfPresent(&f.DeviceType, p.DeviceType)
	setIfPresent(&f.Platform, p.Platform)
	setIfPresent(&f.Browser, p.Browser)
	setIfPresent(&f.Username, p.Username)
	setIfPresent(&f.Fingerprint, p.Fingerprint)
	setIfPresent(&f.FingerprintType, p.FingerprintType)
	return f
}

func trafficFilterParamsFromTimeSeries(p api.GetRequestTimeSeriesParams) trafficFilterParams {
	return trafficFilterParams{
		StartDate:       p.StartDate,
		EndDate:         p.EndDate,
		IsStatic:        p.IsStatic,
		StatusCode:      p.StatusCode,
		Country:         p.Country,
		DeviceType:      p.DeviceType,
		Platform:        p.Platform,
		Browser:         p.Browser,
		Username:        p.Username,
		Fingerprint:     p.Fingerprint,
		FingerprintType: p.FingerprintType,
	}
}

// withDefaultRange fills a missing start or end: end defaults to now and start
// to span before end.
func withDefaultRange(f db.TrafficFilter, span func(end time.Time) time.Time) db.TrafficFilter {
	if f.End == nil {
		now := time.Now()
		f.End = &now
	}
	if f.Start == nil {
		start := span(*f.End)
		f.Start = &start
	}
	return f
}

// dropEmptyKeys removes the "" bucket, which holds requests with no value
// for the dimension (no geo data, no fingerprint, ...).
func dropEmptyKeys(counts map[string]int) map[string]int {
	delete(counts, "")
	return counts
}
