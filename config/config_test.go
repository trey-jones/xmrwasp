package config

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	os.Clearenv()
	oneTimeConfig = sync.Once{}
	instance = nil
}

func testSetRequiredEnvConfigs() {
	os.Setenv("XMRWASP_URL", "fakeurl")
	os.Setenv("XMRWASP_LOGIN", "fakelogin")
	os.Setenv("XMRWASP_PASSWORD", "fakepassword")
	os.Setenv("XMRWASP_NOTCP", "true")
}

func testGetConfigDefaults() (defaultStrings map[string]string, defaultInts map[int]int) {
	defaultStrings = map[string]string{
		instance.WebsocketPort: "8080",
		instance.StratumPort:   "1111",
	}
	defaultInts = map[int]int{
		instance.StatInterval: 60,
		instance.DonateLevel:  3,
	}
	return
}

func testDefaultsAreSet(t *testing.T) {
	defaultStrings, defaultInts := testGetConfigDefaults()
	for value, expectedValue := range defaultStrings {
		if value != expectedValue {
			t.Errorf("%s did not match expected value %s", value, expectedValue)
		}
	}
	for value, expectedValue := range defaultInts {
		if value != expectedValue {
			t.Errorf("%v did not match expected value %v", value, expectedValue)
		}
	}
	if instance.DisableTCP != true {
		t.Errorf("Stratum should be disabled by default.")
	}
}

func TestEnvRequiredConfigs(t *testing.T) {
	defer os.Clearenv()
	// just check for error for now
	err := configFromEnv()
	if err != ErrMissingRequiredConfig {
		t.Error("Expected ErrMissingRequiredConfig and didn't get it.")
	}

	testSetRequiredEnvConfigs()
	err = configFromEnv()
	if err == ErrMissingRequiredConfig {
		t.Error("Got unexpected ErrMissingRequiredConfig")
	}
}

func TestEnvDefaultConfigs(t *testing.T) {
	defer reset()
	testSetRequiredEnvConfigs()
	err := configFromEnv()
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}
	testDefaultsAreSet(t)
}

func TestEnvConfigAccuracy(t *testing.T) {
	defer reset()
	testSetRequiredEnvConfigs()
	os.Setenv("XMRWASP_NOWEB", "true")
	os.Setenv("XMRWASP_STRPORT", "3333")
	os.Setenv("XMRWASP_STATS", "120")
	os.Setenv("XMRWASP_DONATE", "50") // the correct value

	// this also tests the singleton
	require.Equal(t, true, Get().DisableWebsocket)
	require.Equal(t, "3333", Get().StratumPort)
	require.Equal(t, 120, Get().StatInterval)
	require.Equal(t, 50, Get().DonateLevel)
}

func TestFileRequiredConfigs(t *testing.T) {
	defer reset()
	cfg := strings.NewReader("{}")
	err := configFromFile(cfg)
	if err != ErrMissingRequiredConfig {
		t.Error("Expected ErrMissingRequiredConfig and didn't get it.")
	}
}

func TestFileDefaultConfigs(t *testing.T) {
	defer reset()
	cfg := strings.NewReader(`{
        "url": "fakeURL",
        "login": "fakeLogin",
        "password": "fakePassword"
        }`)
	err := configFromFile(cfg)
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}
	testDefaultsAreSet(t)
}

func TestFileConfigAccuracy(t *testing.T) {
	defer reset()
	cfg := strings.NewReader(`{
        "notcp": true,
        "url": "fakeURL",
        "login": "fakeLogin",
        "password": "fakePassword",
        "donate": 80
        }`)
	err := configFromFile(cfg)
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}

	require.Equal(t, true, instance.DisableTCP)
	require.Equal(t, "8080", instance.WebsocketPort) // default
	require.Equal(t, "fakeURL", instance.PoolAddr)
	require.Equal(t, "fakeLogin", instance.PoolLogin)
	require.Equal(t, "fakePassword", instance.PoolPassword)
	require.Equal(t, 60, instance.StatInterval) // default
	require.Equal(t, 80, instance.DonateLevel)
}
