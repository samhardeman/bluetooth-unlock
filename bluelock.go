package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Configuration struct holds all the necessary settings for the application.
type Config struct {
	BluetoothDeviceAddress string        `json:"bluetooth_device_address"`
	CheckInterval          time.Duration `json:"check_interval"`
	CheckRepeat            int           `json:"check_repeat"`
	LockRSSI               int           `json:"lock_rssi"`
	UnlockRSSI             int           `json:"unlock_rssi"`
	DesktopEnv             string        `json:"desktop_env"`
	SessionTimeout         time.Duration `json:"session_timeout"`
	Debug                  bool          `json:"debug"`
}

// DefaultConfig provides default values for the configuration file.
var DefaultConfig = Config{
	BluetoothDeviceAddress: "XX:XX:XX:XX:XX:XX",
	CheckInterval:          5 * time.Second,
	CheckRepeat:            3,
	LockRSSI:               -14,
	UnlockRSSI:             -14,
	DesktopEnv:             "CINNAMON",
	SessionTimeout:         30 * time.Minute, // Default session timeout added
	Debug:                  true,
}

// InitializeConfig initializes configuration values, either from a file or using defaults.
func InitializeConfig() *Config {
	// Check if config.json exists, and create if necessary.
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		if err := WriteDefaultConfig("config.json"); err != nil {
			log.Fatalf("Error creating default config.json: %v", err)
		}
	}

	// Attempt to load configuration from the file.
	config, err := LoadConfig("config.json")
	if err != nil {
		log.Printf("Error loading config.json, using defaults: %v", err)
		return &DefaultConfig
	}

	// Ensure CheckInterval is set correctly, fallback to default if not specified.
	if config.CheckInterval == 0 {
		config.CheckInterval = DefaultConfig.CheckInterval
	}

	// Ensure SessionTimeout is set correctly, fallback to default if not specified.
	if config.SessionTimeout == 0 {
		config.SessionTimeout = DefaultConfig.SessionTimeout
	}

	return config
}

// WriteDefaultConfig creates a config.json file with default settings.
func WriteDefaultConfig(filename string) error {
	// Open or create the config file.
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// Encode default config to JSON with indentation.
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(DefaultConfig); err != nil {
		return fmt.Errorf("failed to encode default config: %w", err)
	}

	log.Println("Default config.json created.")
	return nil
}

// LoadConfig loads configuration from a JSON file.
func LoadConfig(filename string) (*Config, error) {
	// Open the config file.
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	// Create a temporary struct to hold the unmarshalled JSON.
	var tempConfig struct {
		BluetoothDeviceAddress string `json:"bluetooth_device_address"`
		CheckInterval          int    `json:"check_interval"` // Interval in seconds
		CheckRepeat            int    `json:"check_repeat"`
		LockRSSI               int    `json:"lock_rssi"`
		UnlockRSSI             int    `json:"unlock_rssi"`
		DesktopEnv             string `json:"desktop_env"`
		SessionTimeout         int    `json:"session_timeout"` // Expect session timeout in seconds
		Debug                  bool   `json:"debug"`
	}

	// Decode JSON into the temporary struct.
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tempConfig); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Convert tempConfig to the final Config struct.
	config := &Config{
		BluetoothDeviceAddress: tempConfig.BluetoothDeviceAddress,
		CheckInterval:          time.Duration(tempConfig.CheckInterval) * time.Second,
		CheckRepeat:            tempConfig.CheckRepeat,
		LockRSSI:               tempConfig.LockRSSI,
		UnlockRSSI:             tempConfig.UnlockRSSI,
		DesktopEnv:             tempConfig.DesktopEnv,
		SessionTimeout:         time.Duration(tempConfig.SessionTimeout) * time.Second,
		Debug:                  tempConfig.Debug,
	}

	return config, nil
}

// LockSystem locks the system based on desktop environment
func LockSystem(env string) {
	switch env {
	case "LOGINCTL", "KDE":
		exec.Command("loginctl", "lock-session").Run()
	case "GNOME":
		exec.Command("gnome-screensaver-command", "-l").Run()
	case "XSCREENSAVER":
		exec.Command("xscreensaver-command", "-lock").Run()
	case "MATE":
		exec.Command("mate-screensaver-command", "-l").Run()
	case "CINNAMON":
		exec.Command("cinnamon-screensaver-command", "-l").Run()
	}
	fmt.Println("System locked.")
}

// UnlockSystem unlocks the system based on desktop environment
func UnlockSystem(env string) {
	switch env {
	case "LOGINCTL", "KDE":
		exec.Command("loginctl", "unlock-session").Run()
	case "GNOME":
		exec.Command("gnome-screensaver-command", "-d").Run()
	case "XSCREENSAVER":
		exec.Command("pkill", "xscreensaver").Run()
	case "MATE":
		exec.Command("mate-screensaver-command", "-d").Run()
	case "CINNAMON":
		exec.Command("cinnamon-screensaver-command", "-d").Run()
	}
	fmt.Println("System unlocked.")
}

// PingBluetoothDevice uses `hcitool` to check the RSSI of a Bluetooth device for proximity detection.
func PingBluetoothDevice(config *Config) (bool, error) {
	// Run `hcitool` to check RSSI
	cmd := exec.Command("hcitool", "rssi", config.BluetoothDeviceAddress)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// Execute the command and capture the output
	err := cmd.Run()
	if err != nil {
		// If the device is disconnected or `hcitool` fails, catch the error
		fmt.Printf("Error executing hcitool: %s\n", err)
		// Return false to indicate that the device is out of range
		return false, nil
	}

	// Parse the output to find the RSSI value
	output := out.String()
	if strings.Contains(output, "RSSI return value") {
		// Extract the RSSI value from the output
		parts := strings.Split(output, ":")
		if len(parts) < 2 {
			fmt.Println("Unexpected hcitool output format:", output)
			return false, nil
		}
		rssiStr := strings.TrimSpace(parts[1])
		rssi, err := strconv.Atoi(rssiStr)
		if err != nil {
			fmt.Println("Failed to parse RSSI value:", err)
			return false, err
		}

		// Check if RSSI meets the proximity thresholds
		if rssi >= config.UnlockRSSI {
			return true, nil // Device is close enough for unlocking
		} else if rssi <= config.LockRSSI {
			return false, nil // Device is far enough to lock
		}
	}

	// If RSSI not found in output, assume device is out of range
	fmt.Println("Device not found or out of range.")
	return false, nil
}

// MonitorBluetooth monitors the Bluetooth device connection and locks/unlocks based on range.
func MonitorBluetooth(config *Config) {
	mode := "locked"               // Initial state
	lastUnlockedTime := time.Now() // Track the last unlock time

	for {
		// Check if the device is in range using the configured RSSI thresholds
		inRange, err := PingBluetoothDevice(config)
		if err != nil {
			fmt.Println("Error during Bluetooth scan:", err)
			continue
		}

		currentTime := time.Now()

		// If device is in range and was previously locked, unlock it
		if inRange && mode == "locked" {
			UnlockSystem(config.DesktopEnv)
			lastUnlockedTime = currentTime // Update the last unlocked time
			mode = "unlocked"
		} else if !inRange && mode == "unlocked" {
			// If device is out of range and was previously unlocked, lock it
			LockSystem(config.DesktopEnv)
			mode = "locked"
		}

		// If the device is disconnected and the session is unlocked, lock the system
		if !inRange && mode == "unlocked" {
			// Lock system if device is disconnected
			LockSystem(config.DesktopEnv)
			mode = "locked"
		}

		// Check for session timeout
		if mode == "unlocked" && currentTime.Sub(lastUnlockedTime) > config.SessionTimeout {
			fmt.Println("Session timeout reached. Locking system.")
			LockSystem(config.DesktopEnv)
			mode = "locked"
		}

		// Wait before the next check
		time.Sleep(config.CheckInterval)
	}
}

func main() {
	config := InitializeConfig()
	fmt.Println(config.DesktopEnv)
	fmt.Println(config.BluetoothDeviceAddress)
	fmt.Println("Bluetooth-Unlock is now active!")
	MonitorBluetooth(config)
}
