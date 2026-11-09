package conf

// Config is read from the environment.
type Config struct {
	Port      int `envconfig:"PORT"`
	AdminPort int `envconfig:"ADMIN_PORT"`
}
