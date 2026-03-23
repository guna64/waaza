package config

import "os"

type Config struct {
	Port       string
	APIKey     string
	AdminToken string
	Provider   string
	DBDriver   string
	DBDSN      string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	apiKey := os.Getenv("WAAZA_API_KEY")
	if apiKey == "" {
		apiKey = "changeme"
	}
	admin := os.Getenv("WAAZA_ADMIN_TOKEN")
	if admin == "" {
		admin = "adminchangeme"
	}
	provider := os.Getenv("WAAZA_PROVIDER")
	if provider == "" {
		provider = "mock"
	}
	driver := os.Getenv("WAAZA_DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("WAAZA_DB_DSN")
	if dsn == "" {
		dsn = "file:waaza.db?_pragma=foreign_keys(1)"
	}
	return Config{Port: port, APIKey: apiKey, AdminToken: admin, Provider: provider, DBDriver: driver, DBDSN: dsn}
}
