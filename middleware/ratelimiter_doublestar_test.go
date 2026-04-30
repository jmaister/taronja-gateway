package middleware

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

func TestDoublestar(t *testing.T) {
	patterns := []struct {
		Pattern      string
		RequestPath  string
		Expected     bool
		PatternError bool
	}{
		// 1. Basic * vs **
		{Pattern: "/images/*", RequestPath: "/images/cat.jpg", Expected: true, PatternError: false},
		{Pattern: "/images/*", RequestPath: "/images/vacation/cat.jpg", Expected: false, PatternError: false},
		{Pattern: "/images/**", RequestPath: "/images/vacation/cat.jpg", Expected: true, PatternError: false},

		// 2. The PHP extension cases
		{Pattern: "/**/*.php", RequestPath: "/api/v1/index.php", Expected: true, PatternError: false},
		{Pattern: "/**/*.php", RequestPath: "/index.php", Expected: true, PatternError: false},     // requires a subdir
		{Pattern: "/**.php", RequestPath: "/index.php", Expected: true, PatternError: true},        // matches anywhere
		{Pattern: "/**.php", RequestPath: "/images/index.php", Expected: true, PatternError: true}, // matches anywhere

		// 3. Handling "Empty" and Trailing Slashes
		{Pattern: "/api/*", RequestPath: "/api/", Expected: true, PatternError: false},  // * needs a segment
		{Pattern: "/api/*", RequestPath: "/api", Expected: false, PatternError: false},  // * needs a segment
		{Pattern: "/api/**", RequestPath: "/api/", Expected: true, PatternError: false}, // ** matches the base
		{Pattern: "/a/**/b", RequestPath: "/a//b", Expected: true, PatternError: false}, // ** handles empty middle

		// 4. * and ** in the same pattern
		{Pattern: "/download/*/*.zip", RequestPath: "/download/a/b/archive.zip", Expected: false, PatternError: false}, // * needs exactly one segment
		{Pattern: "/download/**/*.zip", RequestPath: "/download/a/b/archive.zip", Expected: true, PatternError: false}, // ** allows multiple segments

		// 5. Case Sensitivity (Critical for Security)
		{Pattern: "/ADMIN/*", RequestPath: "/admin/dashboard", Expected: false, PatternError: false},

		// 6. Partial Wildcards (The "Prefix" test)
		{Pattern: "/v1/auth_*", RequestPath: "/v1/auth_login", Expected: true, PatternError: false},
		{Pattern: "/v1/auth_*", RequestPath: "/v1/auth_login/confirm", Expected: false, PatternError: false},

		// 7. Middle Recursion with Zero Segments
		{Pattern: "/api/**/v1", RequestPath: "/api/v1", Expected: true, PatternError: false},

		// 8. Extension specific recursion
		{Pattern: "/**/*.js", RequestPath: "/scripts/vendor/jquery.min.js", Expected: true, PatternError: false},

		// 9. Root Trailing Slash (The "Clean" vs "Dirty" URL)
		{Pattern: "/**", RequestPath: "/", Expected: true, PatternError: false},
	}

	for _, tc := range patterns {
		matched, err := matchPatterWithDoublestar(tc.Pattern, tc.RequestPath)
		if err != nil {
			if tc.PatternError {
				// Expected pattern error, test passes for this case
				continue
			}
			t.Errorf("Pattern: %s, Request Path: %s, Error: %v", tc.Pattern, tc.RequestPath, err)
			continue
		}
		if matched != tc.Expected {
			t.Errorf("Pattern: %s, Request Path: %s, Expected: %v, Got: %v", tc.Pattern, tc.RequestPath, tc.Expected, matched)
		}
	}
}

func matchPatterWithDoublestar(pattern string, requestPath string) (bool, error) {
	// 1. Validation: Prevent "/**.sh" by ensuring "**" is its own segment
	// We look for "**" and check if it's surrounded by "/" or at string boundaries
	if strings.Contains(pattern, "**") {
		// This regex or logic ensures ** is always /**/ or starts/ends the string properly
		// If it's something like "**.sh", we return an error or handle it
		if err := validatePattern(pattern); err != nil {
			return false, err
		}
	}

	// Normalize separators so matching works consistently on all platforms.
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	requestPath = strings.ReplaceAll(requestPath, "\\", "/")

	matched, err := doublestar.Match(pattern, requestPath)
	if err != nil {
		return false, err
	}
	return matched, nil
}

// validatePattern ensures that "**" is used correctly as a segment.
// This prevents ambiguous patterns like "**.sh" or "admin**".
func validatePattern(pattern string) error {
	if !strings.Contains(pattern, "**") {
		return nil
	}

	segments := strings.Split(pattern, "/")
	for _, segment := range segments {
		// If a segment contains "**" but isn't ONLY "**", it's invalid.
		if strings.Contains(segment, "**") && segment != "**" {
			return fmt.Errorf("invalid pattern '%s': '**' must be a standalone segment like '/**/' or at the start/end of the pattern", pattern)
		}
	}
	return nil
}
