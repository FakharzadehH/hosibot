package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Bot      BotConfig
	API      APIConfig
	Payment  PaymentConfig
	JWT      JWTConfig
}

type ServerConfig struct {
	Port int
	Env  string // "development", "production"
}

type DatabaseConfig struct {
	Host    string
	Port    string
	Name    string
	User    string
	Pass    string
	Charset string
}

type RedisConfig struct {
	Addr string
	Pass string
	DB   int
}

type BotConfig struct {
	Token      string
	WebhookURL string
	UpdateMode string
	AdminID    string
	Username   string
	Domain     string
}

type APIConfig struct {
	Key      string
	HashFile string
}

type PaymentConfig struct {
	ZarinPal      ZarinPalConfig
	NOWPayments   NOWPaymentsConfig
	Tronado       TronadoConfig
	AqayePardakht AqayeConfig
	IranPay       IranPayConfig
}

type ZarinPalConfig struct {
	Merchant string
	Sandbox  bool
}

type NOWPaymentsConfig struct {
	APIKey string
}

type TronadoConfig struct {
	APIKey string
}

type AqayeConfig struct {
	Pin string
}

type IranPayConfig struct {
	APIKey string
}

type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

// Load reads configuration from .env file and environment variables.
func Load() (*Config, error) {
	loadEnvFiles()

	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("APP_PORT", 8080)
	viper.SetDefault("APP_ENV", "production")
	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "3306")
	viper.SetDefault("DB_CHARSET", "utf8mb4")
	viper.SetDefault("REDIS_ADDR", "localhost:6379")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("BOT_UPDATE_MODE", "auto")
	viper.SetDefault("JWT_EXPIRY", "24h")
	viper.SetDefault("ZARINPAL_SANDBOX", false)
	viper.SetDefault("API_HASH_FILE", "hash.txt")

	expiry, err := time.ParseDuration(viper.GetString("JWT_EXPIRY"))
	if err != nil {
		expiry = 24 * time.Hour
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: viper.GetInt("APP_PORT"),
			Env:  viper.GetString("APP_ENV"),
		},
		Database: DatabaseConfig{
			Host:    viper.GetString("DB_HOST"),
			Port:    viper.GetString("DB_PORT"),
			Name:    viper.GetString("DB_NAME"),
			User:    viper.GetString("DB_USER"),
			Pass:    viper.GetString("DB_PASS"),
			Charset: viper.GetString("DB_CHARSET"),
		},
		Redis: RedisConfig{
			Addr: viper.GetString("REDIS_ADDR"),
			Pass: viper.GetString("REDIS_PASS"),
			DB:   viper.GetInt("REDIS_DB"),
		},
		Bot: BotConfig{
			Token:      viper.GetString("BOT_TOKEN"),
			WebhookURL: viper.GetString("BOT_WEBHOOK_URL"),
			UpdateMode: viper.GetString("BOT_UPDATE_MODE"),
			AdminID:    viper.GetString("BOT_ADMIN_ID"),
			Username:   viper.GetString("BOT_USERNAME"),
			Domain:     viper.GetString("BOT_DOMAIN"),
		},
		API: APIConfig{
			Key:      viper.GetString("API_KEY"),
			HashFile: viper.GetString("API_HASH_FILE"),
		},
		Payment: PaymentConfig{
			ZarinPal: ZarinPalConfig{
				Merchant: viper.GetString("ZARINPAL_MERCHANT"),
				Sandbox:  viper.GetBool("ZARINPAL_SANDBOX"),
			},
			NOWPayments: NOWPaymentsConfig{
				APIKey: viper.GetString("NOWPAYMENTS_API_KEY"),
			},
			Tronado: TronadoConfig{
				APIKey: viper.GetString("TRONADO_API_KEY"),
			},
			AqayePardakht: AqayeConfig{
				Pin: viper.GetString("AQAYE_PIN"),
			},
			IranPay: IranPayConfig{
				APIKey: viper.GetString("IRANPAY_API_KEY"),
			},
		},
		JWT: JWTConfig{
			Secret: viper.GetString("JWT_SECRET"),
			Expiry: expiry,
		},
	}

	missing := missingRequiredVars(cfg)
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// LoadDatabaseOnly reads only DB config and validates DB fields required for schema bootstrap.
func LoadDatabaseOnly() (*DatabaseConfig, error) {
	loadEnvFiles()
	viper.AutomaticEnv()

	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "3306")
	viper.SetDefault("DB_CHARSET", "utf8mb4")

	db := &DatabaseConfig{
		Host:    viper.GetString("DB_HOST"),
		Port:    viper.GetString("DB_PORT"),
		Name:    viper.GetString("DB_NAME"),
		User:    viper.GetString("DB_USER"),
		Pass:    viper.GetString("DB_PASS"),
		Charset: viper.GetString("DB_CHARSET"),
	}

	missing := make([]string, 0, 4)
	if strings.TrimSpace(db.Host) == "" {
		missing = append(missing, "DB_HOST")
	}
	if strings.TrimSpace(db.Port) == "" {
		missing = append(missing, "DB_PORT")
	}
	if strings.TrimSpace(db.Name) == "" {
		missing = append(missing, "DB_NAME")
	}
	if strings.TrimSpace(db.User) == "" {
		missing = append(missing, "DB_USER")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables for db bootstrap: %s", strings.Join(missing, ", "))
	}
	return db, nil
}

func loadEnvFiles() {
	candidates := []string{
		".env",
		"go/.env",
		"../.env",
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, ".env"),
			filepath.Join(exeDir, "..", ".env"),
			filepath.Join(exeDir, "..", "..", ".env"),
		)
	}

	seen := make(map[string]bool, len(candidates))
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if err := godotenv.Load(abs); err == nil {
			log.Printf("Loaded environment file: %s", abs)
			return
		}
	}
	log.Println("WARNING: no .env file found in common locations")
}

func missingRequiredVars(cfg *Config) []string {
	missing := make([]string, 0, 8)
	if strings.TrimSpace(cfg.Database.Host) == "" {
		missing = append(missing, "DB_HOST")
	}
	if strings.TrimSpace(cfg.Database.Port) == "" {
		missing = append(missing, "DB_PORT")
	}
	if strings.TrimSpace(cfg.Database.Name) == "" {
		missing = append(missing, "DB_NAME")
	}
	if strings.TrimSpace(cfg.Database.User) == "" {
		missing = append(missing, "DB_USER")
	}
	if strings.TrimSpace(cfg.Bot.Token) == "" {
		missing = append(missing, "BOT_TOKEN")
	}
	if strings.TrimSpace(cfg.API.Key) == "" {
		missing = append(missing, "API_KEY")
	}
	if strings.TrimSpace(cfg.JWT.Secret) == "" {
		missing = append(missing, "JWT_SECRET")
	}
	return missing
}

// DSN returns the MySQL DSN string for GORM.
func (d *DatabaseConfig) DSN() string {
	return d.User + ":" + d.Pass + "@tcp(" + d.Host + ":" + d.Port + ")/" + d.Name + "?charset=" + d.Charset + "&parseTime=True&loc=Local"
}
