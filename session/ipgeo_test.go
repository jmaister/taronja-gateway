package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedGeoCache sets ip's entry in the shared, package-level ipCache
// (GetGeoDataFromIP has no way to inject a per-call cache) and removes
// that one key again when the test completes. Any test that calls
// GetGeoDataFromIP — directly, or transitively via NewClientInfo/
// NewSession/etc. — for an IP not already cached will otherwise make a
// real network call: slow even when it succeeds, and — as found while
// fixing this cache's negative-caching gap — capable of hanging a test
// suite for over an hour when it doesn't (see geoSuccessTTL/geoFailureTTL
// in ipgeo.go). Deleting only this key, rather than resetting the whole
// cache, keeps this from interfering with what other tests running in the
// same process have cached for other IPs.
func seedGeoCache(t *testing.T, ip string, entry geoCacheEntry) {
	t.Helper()
	ipCache.mutex.Lock()
	ipCache.cache[ip] = entry
	ipCache.mutex.Unlock()
	t.Cleanup(func() {
		ipCache.mutex.Lock()
		delete(ipCache.cache, ip)
		ipCache.mutex.Unlock()
	})
}

func TestGetGeoDataFromIP_EmptyIP(t *testing.T) {
	_, err := GetGeoDataFromIP("")
	assert.Error(t, err)
}

func TestGetGeoDataFromIP_Localhost(t *testing.T) {
	data, err := GetGeoDataFromIP("127.0.0.1")
	assert.NoError(t, err)
	assert.Equal(t, GeoData{}, data)
}

// TestGetGeoDataFromIP_NonRoutable is the regression test for the bug a
// real deployment hit: an RFC 1918 private-range client IP
// (192.168.1.60 — a normal address for a client on the same private
// network as a reverse proxy in front of this gateway) was sent straight
// to the geolocation API, which naturally can't answer for it and logged
// "IPLocate returned status code 401" on every single request. The old
// check only ever matched a literal "127." prefix, so every other
// non-routable range — RFC 1918 private, RFC 4193 IPv6 unique-local,
// link-local, unspecified — fell through to a real (always-failing)
// network call. None of these should ever leave the process at all: this
// asserts each one is skipped, and — via the 200ms deadline — that
// they're skipped locally rather than merely receiving a cached failure.
func TestGetGeoDataFromIP_NonRoutable(t *testing.T) {
	ips := []string{
		"192.168.1.60", // the exact address from the reported bug
		"10.0.0.5",
		"172.16.0.1",
		"172.31.255.255",
		"169.254.1.1", // link-local
		"::1",         // IPv6 loopback
		"fc00::1",     // IPv6 unique-local (RFC 4193)
		"fe80::1",     // IPv6 link-local
		"0.0.0.0",     // unspecified
	}
	for _, ip := range ips {
		t.Run(ip, func(t *testing.T) {
			done := make(chan struct{})
			var data GeoData
			var err error
			go func() {
				data, err = GetGeoDataFromIP(ip)
				close(done)
			}()

			select {
			case <-done:
				assert.NoError(t, err)
				assert.Equal(t, GeoData{}, data)
			case <-time.After(200 * time.Millisecond):
				t.Fatal("GetGeoDataFromIP did not return immediately — it likely attempted a real network call for a non-routable address")
			}
		})
	}
}

// TestIsNonRoutable_PublicAddressStillLooksUp confirms the fix didn't
// overreach: a real public IP must still be treated as routable (i.e. a
// genuine geolocation attempt), not swept into the same "skip" bucket as
// private ranges.
func TestIsNonRoutable_PublicAddressStillLooksUp(t *testing.T) {
	assert.False(t, isNonRoutable("8.8.8.8"), "a public IP must not be treated as non-routable")
	assert.False(t, isNonRoutable("203.0.113.1"))
	assert.False(t, isNonRoutable("not-an-ip"), "a malformed value should fall through to a real lookup attempt, not be silently treated as non-routable")
}

// TestGetGeoDataFromIP_CachedFailureIsFastAndReturnsTheSameError is the
// regression test for the bug this cache redesign fixes: a failed lookup
// used to never be cached, so every request from an IP the geolocation API
// couldn't be reached for triggered a fresh network call — and, at the
// full 5s client timeout each, 1,000 requests from the same IP during a
// real outage took over 80 minutes instead of being near-instant after the
// first failure. A cache hit (success or failure) must never reach the
// network, so this test populates the cache directly and relies on a tight
// deadline to prove that: if GetGeoDataFromIP fell through to a real HTTP
// call here, it would blow well past it.
func TestGetGeoDataFromIP_CachedFailureIsFastAndReturnsTheSameError(t *testing.T) {
	const ip = "203.0.113.50" // TEST-NET-3 (RFC 5737) — never a real geo target
	wantErr := fmt.Errorf("geolocation API unreachable")
	seedGeoCache(t, ip, geoCacheEntry{err: wantErr, at: time.Now()})

	done := make(chan struct{})
	var gotErr error
	go func() {
		_, gotErr = GetGeoDataFromIP(ip)
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, wantErr, gotErr)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("GetGeoDataFromIP did not return a cached failure quickly — it likely fell through to a real network call instead of using the cache")
	}
}

// TestGetGeoDataFromIP_CachedSuccessIsFast is the success-path equivalent
// of the failure test above: a fresh cache entry is returned as-is,
// without a network call.
func TestGetGeoDataFromIP_CachedSuccessIsFast(t *testing.T) {
	const ip = "203.0.113.51"
	want := GeoData{City: "Testville", Country: "Testland", CountryCode: "TT"}
	seedGeoCache(t, ip, geoCacheEntry{data: want, at: time.Now()})

	done := make(chan struct{})
	var got GeoData
	var gotErr error
	go func() {
		got, gotErr = GetGeoDataFromIP(ip)
		close(done)
	}()

	select {
	case <-done:
		require.NoError(t, gotErr)
		assert.Equal(t, want, got)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("GetGeoDataFromIP did not return a cached success quickly — it likely fell through to a real network call instead of using the cache")
	}
}

func TestGeoCacheEntry_Expired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		entry   geoCacheEntry
		checkAt time.Time
		want    bool
	}{
		{
			name:    "fresh success is not expired",
			entry:   geoCacheEntry{at: now},
			checkAt: now.Add(geoSuccessTTL - time.Second),
			want:    false,
		},
		{
			name:    "success expires after geoSuccessTTL",
			entry:   geoCacheEntry{at: now},
			checkAt: now.Add(geoSuccessTTL + time.Second),
			want:    true,
		},
		{
			name:    "fresh failure is not expired",
			entry:   geoCacheEntry{err: fmt.Errorf("boom"), at: now},
			checkAt: now.Add(geoFailureTTL - time.Second),
			want:    false,
		},
		{
			name: "failure expires after geoFailureTTL, well before geoSuccessTTL would",
			entry: geoCacheEntry{
				err: fmt.Errorf("boom"), at: now,
			},
			checkAt: now.Add(geoFailureTTL + time.Second),
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.expired(tt.checkAt))
		})
	}
}
