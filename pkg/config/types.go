package config

type Resource struct {
	ID         string                 `yaml:"id" json:"id"`
	Type       string                 `yaml:"type" json:"type"`
	Properties map[string]interface{} `yaml:"properties" json:"properties"`
}

type Config struct {
	Resources []*Resource `yaml:"resources" json:"resources"`
}
