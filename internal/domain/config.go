package domain

type HostConfig struct {
	Alias      string `yaml:"alias"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	User       string `yaml:"user"`
	SSHKeyPath string `yaml:"ssh_key_path"`
}

type Config struct {
	Hosts []HostConfig `yaml:"hosts"`
}
