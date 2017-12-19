package main

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
	configInstance = nil
}

func testSetRequiredEnvConfigs() {
	os.Setenv("XMRWASP_URL", "fakeurl")
	os.Setenv("XMRWASP_LOGIN", "fakelogin")
	os.Setenv("XMRWASP_PASSWORD", "fakepassword")
	os.Setenv("XMRWASP_NOSTRATUM", "true")
}

func testGetConfigDefaults() (defaultStrings map[string]string, defaultInts map[int]int) {
	defaultStrings = map[string]string{
		configInstance.WebsocketPort: "8080",
		configInstance.StratumPort:   "1111",
	}
	defaultInts = map[int]int{
		configInstance.StatInterval: 60,
		configInstance.DonateLevel:  3,
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
	if configInstance.DisableStratum != true {
		t.Errorf("Stratum should be disabled by default.")
	}
}

func TestMain(m *testing.M) {
	setOptions()
	setupLogger()
	os.Exit(m.Run())
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
	require.Equal(t, true, Config().DisableWebsocket)
	require.Equal(t, "3333", Config().StratumPort)
	require.Equal(t, 120, Config().StatInterval)
	require.Equal(t, 50, Config().DonateLevel)
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
        "nostratum": true,
        "url": "fakeURL",
        "login": "fakeLogin",
        "password": "fakePassword",
        "donate": 80
        }`)
	err := configFromFile(cfg)
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}

	require.Equal(t, true, configInstance.DisableStratum)
	require.Equal(t, "8080", configInstance.WebsocketPort) // default
	require.Equal(t, "fakeURL", configInstance.PoolAddr)
	require.Equal(t, "fakeLogin", configInstance.PoolLogin)
	require.Equal(t, "fakePassword", configInstance.PoolPassword)
	require.Equal(t, 60, configInstance.StatInterval) // default
	require.Equal(t, 80, configInstance.DonateLevel)
}
