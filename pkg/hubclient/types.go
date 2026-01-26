package hubclient

import "time"

// Agent represents an agent from the Hub API.
type Agent struct {
	ID              string            `json:"id"`
	AgentID         string            `json:"agentId"`
	Name            string            `json:"name"`
	Template        string            `json:"template,omitempty"`
	GroveID         string            `json:"groveId,omitempty"`
	Grove           string            `json:"grove,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	Status          string            `json:"status"`
	ConnectionState string            `json:"connectionState,omitempty"`
	ContainerStatus string            `json:"containerStatus,omitempty"`
	SessionStatus   string            `json:"sessionStatus,omitempty"`
	RuntimeState    string            `json:"runtimeState,omitempty"`
	Image           string            `json:"image,omitempty"`
	Detached        bool              `json:"detached,omitempty"`
	Runtime         string            `json:"runtime,omitempty"`
	RuntimeHostID   string            `json:"runtimeHostId,omitempty"`
	RuntimeHostType string            `json:"runtimeHostType,omitempty"`
	WebPTYEnabled   bool              `json:"webPtyEnabled,omitempty"`
	TaskSummary     string            `json:"taskSummary,omitempty"`
	AppliedConfig   *AgentConfig      `json:"appliedConfig,omitempty"`
	DirectConnect   *DirectConnect    `json:"directConnect,omitempty"`
	Kubernetes      *KubernetesInfo   `json:"kubernetes,omitempty"`
	Created         time.Time         `json:"created"`
	Updated         time.Time         `json:"updated"`
	LastSeen        time.Time         `json:"lastSeen,omitempty"`
	CreatedBy       string            `json:"createdBy,omitempty"`
	OwnerID         string            `json:"ownerId,omitempty"`
	Visibility      string            `json:"visibility,omitempty"`
	StateVersion    int64             `json:"stateVersion,omitempty"`
}

// AgentConfig represents agent configuration.
type AgentConfig struct {
	Image   string            `json:"image,omitempty"`
	Harness string            `json:"harness,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Model   string            `json:"model,omitempty"`
	Task    string            `json:"task,omitempty"`
}

// DirectConnect contains direct connection info.
type DirectConnect struct {
	Enabled bool   `json:"enabled"`
	SSHHost string `json:"sshHost,omitempty"`
	SSHPort int    `json:"sshPort,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`
}

// KubernetesInfo contains K8s-specific metadata.
type KubernetesInfo struct {
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	PodName   string `json:"podName,omitempty"`
	SyncedAt  string `json:"syncedAt,omitempty"`
}

// Grove represents a grove from the Hub API.
type Grove struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Slug                 string             `json:"slug"`
	GitRemote            string             `json:"gitRemote,omitempty"`
	DefaultRuntimeHostID string             `json:"defaultRuntimeHostId,omitempty"`
	Created              time.Time          `json:"created"`
	Updated              time.Time          `json:"updated"`
	CreatedBy            string             `json:"createdBy,omitempty"`
	OwnerID              string             `json:"ownerId,omitempty"`
	Visibility           string             `json:"visibility,omitempty"`
	Labels               map[string]string  `json:"labels,omitempty"`
	Annotations          map[string]string  `json:"annotations,omitempty"`
	Contributors         []GroveContributor `json:"contributors,omitempty"`
	AgentCount           int                `json:"agentCount,omitempty"`
	ActiveHostCount      int                `json:"activeHostCount,omitempty"`
}

// GroveContributor represents a host contributing to a grove.
type GroveContributor struct {
	HostID    string    `json:"hostId"`
	HostName  string    `json:"hostName"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	Profiles  []string  `json:"profiles,omitempty"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
	LocalPath string    `json:"localPath,omitempty"`
}

// GroveSettings represents grove configuration settings.
type GroveSettings struct {
	ActiveProfile   string                 `json:"activeProfile,omitempty"`
	DefaultTemplate string                 `json:"defaultTemplate,omitempty"`
	Bucket          *BucketConfig          `json:"bucket,omitempty"`
	Runtimes        map[string]interface{} `json:"runtimes,omitempty"`
	Harnesses       map[string]interface{} `json:"harnesses,omitempty"`
	Profiles        map[string]interface{} `json:"profiles,omitempty"`
}

// BucketConfig represents cloud storage configuration.
type BucketConfig struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Prefix   string `json:"prefix,omitempty"`
}

// RuntimeHost represents a runtime host from the Hub API.
type RuntimeHost struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Slug               string            `json:"slug"`
	Type               string            `json:"type"`
	Mode               string            `json:"mode"`
	Version            string            `json:"version"`
	Status             string            `json:"status"`
	ConnectionState    string            `json:"connectionState"`
	LastHeartbeat      time.Time         `json:"lastHeartbeat,omitempty"`
	Capabilities       *HostCapabilities `json:"capabilities,omitempty"`
	SupportedHarnesses []string          `json:"supportedHarnesses,omitempty"`
	Resources          *HostResources    `json:"resources,omitempty"`
	Runtimes           []HostRuntime     `json:"runtimes,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`
	Endpoint           string            `json:"endpoint,omitempty"`
	Groves             []HostGroveInfo   `json:"groves,omitempty"`
	Created            time.Time         `json:"created"`
	Updated            time.Time         `json:"updated"`
}

// HostCapabilities describes runtime host capabilities.
type HostCapabilities struct {
	WebPTY bool `json:"webPty"`
	Sync   bool `json:"sync"`
	Attach bool `json:"attach"`
}

// HostResources describes host resource availability.
type HostResources struct {
	CPUAvailable    string `json:"cpuAvailable,omitempty"`
	MemoryAvailable string `json:"memoryAvailable,omitempty"`
	AgentsRunning   int    `json:"agentsRunning,omitempty"`
	AgentsCapacity  int    `json:"agentsCapacity,omitempty"`
}

// HostRuntime describes a container runtime on a host.
type HostRuntime struct {
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Context   string `json:"context,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// HostGroveInfo describes a grove from a host's perspective.
type HostGroveInfo struct {
	GroveID    string   `json:"groveId"`
	GroveName  string   `json:"groveName"`
	GitRemote  string   `json:"gitRemote,omitempty"`
	Mode       string   `json:"mode"`
	Profiles   []string `json:"profiles,omitempty"`
	AgentCount int      `json:"agentCount"`
	LocalPath  string   `json:"localPath,omitempty"`
}

// Template represents a template from the Hub API.
type Template struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Slug       string          `json:"slug"`
	Harness    string          `json:"harness"`
	Image      string          `json:"image,omitempty"`
	Config     *TemplateConfig `json:"config,omitempty"`
	Scope      string          `json:"scope"`
	GroveID    string          `json:"groveId,omitempty"`
	OwnerID    string          `json:"ownerId,omitempty"`
	Visibility string          `json:"visibility,omitempty"`
	StorageURI string          `json:"storageUri,omitempty"`
	Created    time.Time       `json:"created"`
	Updated    time.Time       `json:"updated"`
}

// TemplateConfig holds template configuration.
type TemplateConfig struct {
	Harness     string            `json:"harness,omitempty"`
	ConfigDir   string            `json:"configDir,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Detached    bool              `json:"detached,omitempty"`
	CommandArgs []string          `json:"commandArgs,omitempty"`
	Model       string            `json:"model,omitempty"`
}

// User represents a user from the Hub API.
type User struct {
	ID          string           `json:"id"`
	Email       string           `json:"email"`
	DisplayName string           `json:"displayName"`
	AvatarURL   string           `json:"avatarUrl,omitempty"`
	Role        string           `json:"role"`
	Status      string           `json:"status"`
	Preferences *UserPreferences `json:"preferences,omitempty"`
	Created     time.Time        `json:"created"`
	LastLogin   time.Time        `json:"lastLogin,omitempty"`
}

// UserPreferences holds user preferences.
type UserPreferences struct {
	DefaultTemplate string `json:"defaultTemplate,omitempty"`
	DefaultProfile  string `json:"defaultProfile,omitempty"`
	Theme           string `json:"theme,omitempty"`
}

// EnvVar represents an environment variable from the Hub API.
type EnvVar struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Scope       string    `json:"scope"`
	ScopeID     string    `json:"scopeId"`
	Description string    `json:"description,omitempty"`
	Sensitive   bool      `json:"sensitive,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	CreatedBy   string    `json:"createdBy,omitempty"`
}

// Secret represents secret metadata from the Hub API.
// Note: Secret values are never returned by the API.
type Secret struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Scope       string    `json:"scope"`
	ScopeID     string    `json:"scopeId"`
	Description string    `json:"description,omitempty"`
	Version     int       `json:"version"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	CreatedBy   string    `json:"createdBy,omitempty"`
	UpdatedBy   string    `json:"updatedBy,omitempty"`
}
