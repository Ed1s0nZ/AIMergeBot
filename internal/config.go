package internal

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	GitLab struct {
		Token string `yaml:"token"`
		URL   string `yaml:"url"`
	} `yaml:"gitlab"`
	OpenAI struct {
		APIKey string `yaml:"api_key"`
		URL    string `yaml:"url"`
		Model  string `yaml:"model"`
	} `yaml:"openai"`
	Projects            []ProjectConfig `yaml:"projects"`
	EnableWebhook       bool            `yaml:"enable_webhook"`
	EnableMRComment     bool            `yaml:"enable_mr_comment"`
	ScanExistingMRs     bool            `yaml:"scan_existing_mrs"`
	EnablePolling       bool            `yaml:"enable_polling"`
	WhitelistExtensions []string        `yaml:"whitelist_extensions"`
	MCP                 MCPConfig       `yaml:"mcp"`
	ReAct               ReActConfig     `yaml:"react"`
}

type ProjectConfig struct {
	ID      int    `yaml:"id"`
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

type MCPConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Mode     string `yaml:"mode"` // full, simplified, hybrid
	MaxSteps int    `yaml:"max_steps"`
	Verbose  bool   `yaml:"verbose_logging"`
}

type ReActConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxRetries  int     `yaml:"max_retries"`
	MaxSteps    int     `yaml:"max_steps"`
}

// GetMCPMode 获取MCP模式，提供默认值
func (c *MCPConfig) GetMCPMode() string {
	if c.Mode == "" {
		return "simplified" // 默认使用简化模式
	}
	return c.Mode
}

// IsSimplifiedMode 检查是否为简化模式
func (c *MCPConfig) IsSimplifiedMode() bool {
	return c.GetMCPMode() == "simplified"
}

// IsFullMode 检查是否为完整模式
func (c *MCPConfig) IsFullMode() bool {
	return c.GetMCPMode() == "full"
}

// IsHybridMode 检查是否为混合模式
func (c *MCPConfig) IsHybridMode() bool {
	return c.GetMCPMode() == "hybrid"
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
