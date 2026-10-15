// Package featureflag holds the feature flag service: CRUD for the admin API
// and a cached snapshot of flag values for the public API and the
// X-Feature-Flags header the proxy sends to backends.
package featureflag

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"gorm.io/gorm"
)

// HeaderName is the request header proxied requests carry: the keys of every
// enabled flag, sorted and comma-separated. Client-supplied copies are dropped.
const HeaderName = "X-Feature-Flags"

var (
	ErrInvalidKey    = errors.New("invalid feature flag key: use 1-100 lowercase letters, digits, '-', '_' or '.', starting with a letter or digit")
	ErrInvalidType   = fmt.Errorf("invalid feature flag type: only %q is supported", db.FeatureFlagTypeBoolean)
	ErrAlreadyExists = errors.New("feature flag already exists")
	ErrNotFound      = errors.New("feature flag not found")
)

// keyPattern keeps keys safe to list in a comma-separated header and to use
// as URL path segments.
var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// ValidateKey reports whether key is an acceptable flag key.
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return ErrInvalidKey
	}
	return nil
}

// Service manages feature flags. Values are cached in memory and the cache is
// dropped on every write made through the Service, so reads on the request
// path cost no query once warm.
type Service struct {
	repo repositories.FeatureFlagRepository

	mu         sync.RWMutex
	values     map[string]bool // nil until loaded
	generation uint64          // bumped by every write
}

// NewService creates a Service backed by repo.
func NewService(repo repositories.FeatureFlagRepository) *Service {
	return &Service{repo: repo}
}

// List returns every flag, ordered by key.
func (s *Service) List() ([]db.FeatureFlag, error) {
	return s.repo.List()
}

// Get returns one flag, or ErrNotFound.
func (s *Service) Get(key string) (*db.FeatureFlag, error) {
	flag, err := s.repo.Get(key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return flag, err
}

// CreateInput describes a new flag. An empty Type means boolean.
type CreateInput struct {
	Key         string
	Description string
	Type        string
	Enabled     bool
}

// Create stores a new flag.
func (s *Service) Create(in CreateInput) (*db.FeatureFlag, error) {
	key := strings.TrimSpace(in.Key)
	err := ValidateKey(key)
	if err != nil {
		return nil, err
	}
	flagType := in.Type
	if flagType == "" {
		flagType = db.FeatureFlagTypeBoolean
	}
	if flagType != db.FeatureFlagTypeBoolean {
		return nil, ErrInvalidType
	}

	_, err = s.repo.Get(key)
	if err == nil {
		return nil, ErrAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	flag := &db.FeatureFlag{
		Key:         key,
		Description: in.Description,
		Type:        flagType,
		Enabled:     in.Enabled,
	}
	err = s.repo.Create(flag)
	if err != nil {
		return nil, err
	}
	s.invalidate()
	return flag, nil
}

// UpdateInput holds the fields to change; nil fields keep their value.
type UpdateInput struct {
	Description *string
	Enabled     *bool
}

// Update changes an existing flag, or returns ErrNotFound.
func (s *Service) Update(key string, in UpdateInput) (*db.FeatureFlag, error) {
	flag, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	if in.Description != nil {
		flag.Description = *in.Description
	}
	if in.Enabled != nil {
		flag.Enabled = *in.Enabled
	}
	err = s.repo.Update(flag)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.invalidate()
	return s.Get(key)
}

// Delete removes a flag, or returns ErrNotFound.
func (s *Service) Delete(key string) error {
	err := s.repo.Delete(key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// Values returns every flag's current value, keyed by flag key. The map is a
// copy the caller may keep.
func (s *Service) Values() (map[string]bool, error) {
	values, err := s.cachedValues()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out, nil
}

// HeaderValue returns the X-Feature-Flags value: enabled keys, sorted,
// comma-separated. Empty when no flag is on.
func (s *Service) HeaderValue() (string, error) {
	values, err := s.cachedValues()
	if err != nil {
		return "", err
	}
	enabled := make([]string, 0, len(values))
	for k, on := range values {
		if on {
			enabled = append(enabled, k)
		}
	}
	sort.Strings(enabled)
	return strings.Join(enabled, ","), nil
}

func (s *Service) cachedValues() (map[string]bool, error) {
	s.mu.RLock()
	values := s.values
	generation := s.generation
	s.mu.RUnlock()
	if values != nil {
		return values, nil
	}

	flags, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	values = make(map[string]bool, len(flags))
	for _, f := range flags {
		values[f.Key] = f.Enabled
	}

	// A write that landed while List ran bumped the generation; caching this
	// load then could keep its stale values until the next write.
	s.mu.Lock()
	if s.generation == generation {
		s.values = values
	}
	s.mu.Unlock()
	return values, nil
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.values = nil
	s.generation++
	s.mu.Unlock()
}
