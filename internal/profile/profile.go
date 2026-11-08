package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// Profile represents credential and network configuration for an NGA account.
type Profile struct {
	Name      string `yaml:"-"`
	CookieUID int64  `yaml:"cookie_uid"`
	CookieCID string `yaml:"cookie_cid"`
	Proxy     string `yaml:"proxy,omitempty"`
	UserAgent string `yaml:"user_agent,omitempty"`
}

// ConfigFile represents the YAML file schema for profiles and active selection.
type ConfigFile struct {
	CurrentProfile string             `yaml:"current_profile"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

// Manager manages profiles stored in the local config file.
type Manager struct {
	path string
	mu   sync.RWMutex
	cfg  ConfigFile
}

// NewManager initializes a profile manager pointing to config file path.
func NewManager(path string) (*Manager, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".config", "nga", "profiles.yaml")
	}

	m := &Manager{
		path: path,
		cfg: ConfigFile{
			CurrentProfile: "default",
			Profiles:       make(map[string]Profile),
		},
	}

	_ = m.Load()
	return m, nil
}

// Load reads and unmarshals the profile config file.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	m.cfg = cfg
	return nil
}

// Save marshals and writes the profile configuration back to disk.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(&m.cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(m.path, data, 0600)
}

// Get returns the named profile, or the active profile if name is empty.
func (m *Manager) Get(name string) (Profile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		name = m.cfg.CurrentProfile
	}
	p, ok := m.cfg.Profiles[name]
	if ok {
		p.Name = name
	}
	return p, ok
}

// Set adds or updates a profile.
func (m *Manager) Set(p Profile) error {
	if p.Name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	m.mu.Lock()
	if m.cfg.Profiles == nil {
		m.cfg.Profiles = make(map[string]Profile)
	}
	name := p.Name
	m.cfg.Profiles[name] = p
	if m.cfg.CurrentProfile == "" {
		m.cfg.CurrentProfile = name
	}
	m.mu.Unlock()

	return m.Save()
}

// Use sets the active profile name.
func (m *Manager) Use(name string) error {
	m.mu.Lock()
	if _, ok := m.cfg.Profiles[name]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("profile %q does not exist", name)
	}
	m.cfg.CurrentProfile = name
	m.mu.Unlock()

	return m.Save()
}

// Remove deletes a profile by name.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	delete(m.cfg.Profiles, name)
	if m.cfg.CurrentProfile == name {
		m.cfg.CurrentProfile = ""
		for k := range m.cfg.Profiles {
			m.cfg.CurrentProfile = k
			break
		}
	}
	m.mu.Unlock()

	return m.Save()
}

// List returns all profiles sorted by name, along with the active profile name.
func (m *Manager) List() ([]Profile, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for k := range m.cfg.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)

	var list []Profile
	for _, k := range names {
		p := m.cfg.Profiles[k]
		p.Name = k
		list = append(list, p)
	}
	return list, m.cfg.CurrentProfile
}
