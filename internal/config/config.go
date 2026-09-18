package config

import (
	"fmt"
	"os"
)

type Credentials struct {
	AppID     string
	APIKey    string
	APISecret string
}

func (c *Credentials) FromEnv() {
	if c.AppID == "" {
		c.AppID = os.Getenv("XFYUN_APP_ID")
	}
	if c.APIKey == "" {
		c.APIKey = os.Getenv("XFYUN_API_KEY")
	}
	if c.APISecret == "" {
		c.APISecret = os.Getenv("XFYUN_API_SECRET")
	}
}

func (c Credentials) ValidateSigned() error {
	if c.AppID == "" || c.APIKey == "" || c.APISecret == "" {
		return fmt.Errorf("missing credentials: set XFYUN_APP_ID, XFYUN_API_KEY and XFYUN_API_SECRET")
	}
	return nil
}

func (c Credentials) ValidateTTS() error {
	return c.ValidateSigned()
}
