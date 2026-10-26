package config

import (
	"fmt"
	"lensamity/internal/storage"
	"os"
	"strconv"
)

type Config struct {
	SessionSecret string
	DatabaseURL   string
	APIPort       string
	S3            storage.Config
}

func Load() (Config, error) {
	secret, err := LoadAuth()
	if err != nil {
		return Config{}, err
	}

	dbURL, err := LoadDB()
	if err != nil {
		return Config{}, err
	}

	confS3, err := LoadS3()
	if err != nil {
		return Config{}, err
	}

	return Config{
		SessionSecret: secret,
		DatabaseURL:   dbURL,
		APIPort:       withDefault("API_PORT", "8080"),
		S3:            confS3,
	}, nil
}

func LoadAuth() (string, error) {
	secret, err := required("SESSION_SECRET")
	if err != nil {
		return "", err
	}

	return secret, nil
}

func LoadDB() (string, error) {
	return required("DATABASE_URL")
}

func LoadS3() (storage.Config, error) {
	region, err := required("S3_REGION")
	if err != nil {
		return storage.Config{}, err
	}

	accessID, err := required("S3_ACCESS_KEY_ID")
	if err != nil {
		return storage.Config{}, err
	}

	secretAccess, err := required("S3_SECRET_ACCESS_KEY")
	if err != nil {
		return storage.Config{}, err
	}

	internalEndpoint, err := required("S3_INTERNAL_ENDPOINT")
	if err != nil {
		return storage.Config{}, err
	}

	publicEndpoint, err := required("S3_PUBLIC_ENDPOINT")
	if err != nil {
		return storage.Config{}, err
	}

	bucket, err := required("S3_BUCKET")
	if err != nil {
		return storage.Config{}, err
	}

	pathStyle, err := strconv.ParseBool(withDefault("S3_FORCE_PATH_STYLE", "true"))
	if err != nil {
		return storage.Config{}, err
	}

	return storage.Config{
		Region:           region,
		AccessKeyID:      accessID,
		SecretAccessKey:  secretAccess,
		InternalEndpoint: internalEndpoint,
		PublicEndpoint:   publicEndpoint,
		Bucket:           bucket,
		UsePathStyle:     pathStyle,
	}, nil
}

func required(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("required environment variable %s doesn't exist", key)
	}
	return val, nil
}

func withDefault(key, defaultKey string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultKey
	}
	return val
}
