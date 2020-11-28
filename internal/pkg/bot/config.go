package bot

type grpcClientConfig struct {
	Host string `yaml:"host"`
	Port int32  `yaml:"port"`
	Tls  struct {
		Enabled bool   `yaml:"enabled"`
		Name    string `yaml:"name"`
		Ca      string `yaml:"ca"`
		Cert    string `yaml:"cert"`
		Key     string `yaml:"key"`
	} `yaml:"tls"`
}
