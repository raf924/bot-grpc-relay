package config

type GrpcServerConfig struct {
	Port int32 `yaml:"port"`
	//Duration string: 22s, 44m, 3h2m
	Timeout string `yaml:"timeout"`
	TLS     struct {
		Enabled bool     `yaml:"enabled"`
		Ca      string   `yaml:"ca"`
		Cert    string   `yaml:"cert"`
		Key     string   `yaml:"key"`
		Users   []string `yaml:"users"`
	} `yaml:"tls"`
}
