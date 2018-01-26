package config

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO test choose env or file

func reset() {
	os.Clearenv()
	instantiation = sync.Once{}
	instance = nil
}

func testSetRequiredEnvConfigs() {
	os.Setenv("XMRWASP_URL", "fakeurl")
	os.Setenv("XMRWASP_LOGIN", "fakelogin")
	os.Setenv("XMRWASP_PASSWORD", "fakepassword")
	os.Setenv("XMRWASP_NOTCP", "true")
}

func testGetConfigDefaults() (defaultStrings map[string]string, defaultInts map[int]int, defaultBools map[bool]bool) {
	defaultStrings = map[string]string{}
	defaultInts = map[int]int{
		instance.StatInterval:  60,
		instance.DonateLevel:   2,
		instance.WebsocketPort: 8080,
		instance.StratumPort:   1111,
	}
	defaultBools = map[bool]bool{
		instance.DisableWebsocket: false,
		instance.DisableTCP:       true,
		instance.SecureWebsocket:  false,
		instance.DiscardLog:       false,
	}
	return
}

func testDefaultsAreSet(t *testing.T) {
	defaultStrings, defaultInts, defaultBools := testGetConfigDefaults()
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
	for value, expectedValue := range defaultBools {
		if value != expectedValue {
			t.Errorf("%v did not match expected value %v", value, expectedValue)
		}
	}
}

func TestEnvRequiredConfigs(t *testing.T) {
	defer os.Clearenv()
	// just check for error for now
	err := configFromEnv()
	if err == nil || !IsMissingConfig(err) {
		t.Error("Expected config error and got: ", err)
	}

	testSetRequiredEnvConfigs()
	err = configFromEnv()
	if err != nil {
		t.Error("Got unexpected error: ", err)
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
	require.Equal(t, 3333, Get().StratumPort)
	require.Equal(t, 120, Get().StatInterval)
	require.Equal(t, 50, Get().DonateLevel)
}

func TestFileRequiredConfigs(t *testing.T) {
	defer reset()
	cfg := strings.NewReader("{}")
	err := configFromFile(cfg)
	if err == nil || !IsMissingConfig(err) {
		t.Error("Expected config error and got: ", err)
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
        "notcp": false,
        "url": "fakeURL",
        "login": "fakeLogin",
        "password": "fakePassword",
        "donate": 80
        }`)
	err := configFromFile(cfg)
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}

	require.Equal(t, false, instance.DisableTCP)
	require.Equal(t, 8080, instance.WebsocketPort) // default
	require.Equal(t, "fakeURL", instance.PoolAddr)
	require.Equal(t, "fakeLogin", instance.PoolLogin)
	require.Equal(t, "fakePassword", instance.PoolPassword)
	require.Equal(t, 60, instance.StatInterval) // default
	require.Equal(t, 80, instance.DonateLevel)
}

func TestFileConfigAccuracyAgain(t *testing.T) {
	defer reset()
	cfg := strings.NewReader(`{
		"debug": true,
		"noweb": true,
        "notcp": false,
        "wsport": 9125,
        "strport": 1800,
        "wss": false,
        "url": "fakeURL",
        "login": "fakeLogin",
        "password": "fakePassword",
        "log": "proxy.log"
        }`)
	err := configFromFile(cfg)
	if err != nil {
		t.Error("Got unexpected config error: ", err)
	}

	require.Equal(t, true, instance.Debug)
	require.Equal(t, true, instance.DisableWebsocket)
	require.Equal(t, false, instance.DisableTCP)
	require.Equal(t, 9125, instance.WebsocketPort)
	require.Equal(t, 1800, instance.StratumPort)
	require.Equal(t, false, instance.SecureWebsocket)
	require.Equal(t, "fakeURL", instance.PoolAddr)
	require.Equal(t, "fakeLogin", instance.PoolLogin)
	require.Equal(t, "fakePassword", instance.PoolPassword)
	require.Equal(t, 60, instance.StatInterval) // default
	require.Equal(t, 2, instance.DonateLevel)   // default
	require.Equal(t, "proxy.log", instance.LogFile)
}
