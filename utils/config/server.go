package config

// ServerConfig holds configuration for the HTTP server
type ServerConfig struct {
	Port         int                `yaml:"port"`
	DataDir      string             `yaml:"dataDir"`
	RuntimeDir   string             `yaml:"runtimeDir"` // Directory for runtime files like uploads and YAML processing
	Enabled      bool               `yaml:"enabled"`
	BearerToken  string             `yaml:"bearerToken"`
	CORS         CORS               `yaml:"cors"`
	OpenAICompat OpenAICompatConfig `yaml:"openai_compat,omitempty"`
}

// CORS holds Cross-Origin Resource Sharing settings
type CORS struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowedOrigins"`
	AllowedMethods []string `yaml:"allowedMethods"`
	AllowedHeaders []string `yaml:"allowedHeaders"`
	MaxAge         int      `yaml:"maxAge"`
}

// OpenAICompatConfig holds OpenAI API compatibility settings
type OpenAICompatConfig struct {
	Enabled bool   `yaml:"enabled"`
	Prefix  string `yaml:"prefix"` // API prefix, default: "/v1"
}
