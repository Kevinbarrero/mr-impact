package conf

// Config is loaded from the environment.
type Config struct {
	Port    int    `envconfig:"PORT" default:"80"`
	OldKey  string `envconfig:"OLD_KEY"`
	Debug   bool   `yaml:"debug"`
	Written string `env:"WRITTEN"`
	Plain   string
	JSON    string `json:"json"`
}

// Port is read.
func Use(c Config) int {
	c.Written = "x"
	return c.Port
}
