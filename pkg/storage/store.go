package storage

import (
	"encoding/json"
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
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

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

// All returns a snapshot of all known tags sorted: named tags first
// (alphabetically), then unnamed tags (by MAC).
func (s *Store) All() []*Tag {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Tag, 0, len(s.tags))
	for _, t := range s.tags {
		cp := *t
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		ni, nj := out[i].Name != "", out[j].Name != ""
		if ni != nj {
			return ni // named tags first
		}
		if ni {
			return out[i].Name < out[j].Name
		}
		return out[i].MAC < out[j].MAC
	})

	return out
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
	return os.WriteFile(s.path, data, 0644)
}
