package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

// Tag holds the latest known data for one Ruuvi Tag.
type Tag struct {
	MAC         string    `json:"mac"`
	Name        string    `json:"name,omitempty"`
	Selected    bool      `json:"selected"`
	SortOrder   int       `json:"sortOrder,omitempty"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	Pressure    float64   `json:"pressure"`
	LastSeen    time.Time `json:"lastSeen"`
	Status      string    `json:"status"`
}

// DisplayName returns the user-given name, or the MAC if no name is set.
func (t *Tag) DisplayName() string {
	if t.Name != "" {
		return t.Name
	}
	return t.MAC
}

// Store persists tag names to a JSON file and holds the latest sensor data
// in memory. Names survive restarts; sensor data is refreshed by BLE scanning.
type Store struct {
	mu       sync.RWMutex
	tags     map[string]*Tag
	path     string
	onChange func()
}

// NewStore loads existing tags from path (creates an empty store if the file
// does not exist yet).
func NewStore(path string) (*Store, error) {
	s := &Store{
		tags: make(map[string]*Tag),
		path: path,
	}
	if err := s.load(); err != nil {
		if os.IsNotExist(err) {
			log.Printf("store: no existing tags file at %s, starting fresh", path)
		} else {
			return nil, err
		}
	} else {
		log.Printf("store: loaded %d tags from %s", len(s.tags), path)
	}
	// Write immediately to verify the path is writable and to create the file.
	if err := s.save(); err != nil {
		return nil, fmt.Errorf("store: cannot write to %s: %w", path, err)
	}
	return s, nil
}

// Path returns the file path where tags are persisted.
func (s *Store) Path() string { return s.path }

// SetOnChange registers a callback that is invoked after any in-memory update.
// Safe to call before or after scanning starts.
func (s *Store) SetOnChange(fn func()) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// UpdateFromBLE records the latest sensor reading for mac. If the tag is new
// it is added with no name; existing names are never overwritten.
func (s *Store) UpdateFromBLE(mac string, temp, hum, pres float64) {
	s.mu.Lock()
	tag, ok := s.tags[mac]
	if !ok {
		tag = &Tag{MAC: mac, Status: "active", Selected: true}
		s.tags[mac] = tag
	}
	tag.Temperature = temp
	tag.Humidity = hum
	tag.Pressure = pres
	tag.LastSeen = time.Now().UTC()
	tag.Status = "active"
	cb := s.onChange
	s.mu.Unlock()

	if cb != nil {
		cb()
	}
}

// SetName assigns a human-readable name to mac and persists the change.
// Pass an empty string to clear the name.
func (s *Store) SetName(mac, name string) error {
	s.mu.Lock()
	if tag, ok := s.tags[mac]; ok {
		tag.Name = name
	}
	s.mu.Unlock()
	return s.save()
}

// SetSelected marks mac as included or excluded from webhook sends and persists the change.
func (s *Store) SetSelected(mac string, selected bool) error {
	s.mu.Lock()
	if tag, ok := s.tags[mac]; ok {
		tag.Selected = selected
	}
	s.mu.Unlock()
	return s.save()
}

// AllSelected returns the same sorted snapshot as All but limited to tags
// where Selected is true.
func (s *Store) AllSelected() []*Tag {
	all := s.All()
	out := all[:0]
	for _, t := range all {
		if t.Selected {
			out = append(out, t)
		}
	}
	return out
}

// All returns a snapshot of all known tags. If any tag has a SortOrder set,
// the list is sorted by SortOrder (custom user order). Otherwise tags are
// sorted named-first alphabetically, then unnamed by MAC.
func (s *Store) All() []*Tag {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Tag, 0, len(s.tags))
	for _, t := range s.tags {
		cp := *t
		out = append(out, &cp)
	}

	sortSlice(out)
	return out
}

// Move shifts the tag identified by mac one position in the given direction
// (delta = -1 for up, +1 for down) within the sorted list and persists the change.
func (s *Store) Move(mac string, delta int) error {
	s.mu.Lock()

	// Build sorted slice under the write lock (same logic as All).
	tags := make([]*Tag, 0, len(s.tags))
	for _, t := range s.tags {
		tags = append(tags, t)
	}
	sortSlice(tags)

	// Find the target tag.
	idx := -1
	for i, t := range tags {
		if t.MAC == mac {
			idx = i
			break
		}
	}

	if idx < 0 {
		s.mu.Unlock()
		return nil
	}

	newIdx := idx + delta
	if newIdx < 0 || newIdx >= len(tags) {
		s.mu.Unlock()
		return nil
	}

	// Assign sequential SortOrder to all tags so they are fully explicit.
	for i, t := range tags {
		t.SortOrder = i + 1
	}
	// Swap the two positions.
	tags[idx].SortOrder, tags[newIdx].SortOrder = tags[newIdx].SortOrder, tags[idx].SortOrder

	s.mu.Unlock()
	return s.save()
}

// sortSlice sorts a slice of Tag pointers in-place: by SortOrder when any tag
// has a non-zero value, otherwise named tags first (alphabetical) then by MAC.
func sortSlice(tags []*Tag) {
	customOrder := false
	for _, t := range tags {
		if t.SortOrder > 0 {
			customOrder = true
			break
		}
	}

	if customOrder {
		n := len(tags)
		sort.Slice(tags, func(i, j int) bool {
			oi, oj := tags[i].SortOrder, tags[j].SortOrder
			if oi == 0 {
				oi = n + 1
			}
			if oj == 0 {
				oj = n + 1
			}
			return oi < oj
		})
		return
	}

	sort.Slice(tags, func(i, j int) bool {
		ni, nj := tags[i].Name != "", tags[j].Name != ""
		if ni != nj {
			return ni // named tags first
		}
		if ni {
			return tags[i].Name < tags[j].Name
		}
		return tags[i].MAC < tags[j].MAC
	})
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var tags map[string]*Tag
	if err := json.Unmarshal(data, &tags); err != nil {
		return err
	}
	s.tags = tags
	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.tags, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("store: failed to save to %s: %v", s.path, err)
		return err
	}
	log.Printf("store: saved to %s", s.path)
	return nil
}
