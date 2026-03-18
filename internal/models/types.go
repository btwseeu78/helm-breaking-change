package models

// Dependency represents a subchart dependency entry from Chart.yaml.
type Dependency struct {
	Name       string   `yaml:"name"`
	Alias      string   `yaml:"alias,omitempty"`
	Version    string   `yaml:"version"`
	Repository string   `yaml:"repository"`
	Condition  string   `yaml:"condition,omitempty"`
	Tags       []string `yaml:"tags,omitempty"`
}

// ResolveKey returns the alias if set, otherwise the name.
// This is the key used in the parent values.yaml.
func (d Dependency) ResolveKey() string {
	if d.Alias != "" {
		return d.Alias
	}
	return d.Name
}

// VersionChange holds old and new versions for a changed dependency.
type VersionChange struct {
	Dependency Dependency
	OldVersion string
	NewVersion string
}

// ChangeType classifies the nature of a detected diff.
type ChangeType int

const (
	ChangeStructural        ChangeType = iota // scalar↔map↔slice, depth change
	ChangeKeyRemoved                          // key removed in subchart source, parent overrides it
	ChangeKeyAdded                            // new key in subchart source (informational)
	ChangeValueOnly                           // same key/type, different value
	ChangeSafeTypeConv                        // lossless type conversion
	ChangeParentTypeMismatch                  // parent override type incompatible with subchart source type
	ChangeMissingOverride                     // subchart source key exists in parent-overridden block but parent doesn't set it
	ChangeKeyOrphanOverride                   // parent overrides a key that doesn't exist in upstream subchart source (breaking — override is ineffective)
)

// DiffResult holds one detected difference for a key path.
type DiffResult struct {
	KeyPath  string
	Type     ChangeType
	Breaking bool
	OldValue interface{}
	NewValue interface{}
	Detail   string
}

// SubchartReport aggregates diff results for a single subchart.
type SubchartReport struct {
	SubchartName string
	OldVersion   string
	NewVersion   string
	Results      []DiffResult
}

// Report aggregates all subchart reports for the entire run.
type Report struct {
	ChartName       string
	TargetBranch    string
	SubchartReports []SubchartReport
	HasBreaking     bool
}

// RepoAuth holds credentials for protected Helm repositories (HTTP and OCI).
type RepoAuth struct {
	Username         string
	Password         string
	Token            string
	DockerConfigPath string // Path to Docker config.json for OCI registry auth
	RegistryCAFile   string // CA certificate for private OCI registries
}

// HasCredentials returns true if any auth fields are populated.
func (a *RepoAuth) HasCredentials() bool {
	if a == nil {
		return false
	}
	return a.Username != "" || a.Password != "" || a.Token != "" || a.DockerConfigPath != ""
}
