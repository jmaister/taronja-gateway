package handlers

import (
	"time"
)

// FormatDate formats a time.Time, *time.Time, or string to a readable string for templates.
func FormatDate(t interface{}) string {
	if t == nil {
		return ""
	}
	var tm time.Time
	switch v := t.(type) {
	case time.Time:
		tm = v
	case *time.Time:
		if v == nil {
			return ""
		}
		tm = *v
	case string:
		parsed, err := time.Parse(time.RFC1123, v)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, v)
			if err != nil {
				return v // fallback: return as-is
			}
		}
		tm = parsed
	default:
		return ""
	}
	return tm.Format("2006-01-02 15:04:05")
}
