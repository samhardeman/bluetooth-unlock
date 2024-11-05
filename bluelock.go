package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"tinygo.org/x/bluetooth"
)

// Configuration struct to hold all settings
type Config struct {
	BluetoothDeviceAddress string        `json:"bluetooth_device_address"`
	CheckInterval          time.Duration `json:"check_interval"`
	CheckRepeat            int           `json:"check_repeat"`
	DesktopEnv             string        `json:"desktop_env"`
	Debug                  bool          `json:"debug"`
}

var adapter = bluetooth.DefaultAdapter

// DefaultConfig provides default values for the configuration file.
var DefaultConfig = Config{
	BluetoothDeviceAddress: "XX:XX:XX:XX:XX:XX",
	CheckInterval:          5 * time.Second,
	CheckRepeat:            3,
	DesktopEnv:             "CINNAMON",
	Debug:                  true,
}

// InitializeConfig initializes configuration values, either from a file or using defaults.
func InitializeConfig() *Config {
	// Check if config.json exists
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		fmt.Println("Config file not found. Creating default config.json...")
		if err := WriteDefaultConfig("config.json"); err != nil {
			fmt.Println("Error creating default config.json:", err)
		}
	}

	// Load the configuration from the file
	config, err := LoadConfig("config.json")
	if err != nil {
		fmt.Println("Error loading config.json, using defaults:", err)
		return &DefaultConfig
	}

	// Set a default CheckInterval if it's not specified in the config file
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Second
	}

	return config
}

// WriteDefaultConfig creates a config.json file with default settings.
func WriteDefaultConfig(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Convert CheckInterval to seconds for JSON
	tempConfig := struct {
		BluetoothDeviceAddress string `json:"bluetooth_device_address"`
		CheckInterval          int    `json:"check_interval"` // Interval in seconds
		CheckRepeat            int    `json:"check_repeat"`
		DesktopEnv             string `json:"desktop_env"`
		Debug                  bool   `json:"debug"`
	}{
		BluetoothDeviceAddress: DefaultConfig.BluetoothDeviceAddress,
		CheckInterval:          int(DefaultConfig.CheckInterval.Seconds()),
		CheckRepeat:            DefaultConfig.CheckRepeat,
		DesktopEnv:             DefaultConfig.DesktopEnv,
		Debug:                  DefaultConfig.Debug,
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Format JSON with indentation
	if err := encoder.Encode(tempConfig); err != nil {
		return err
	}

	fmt.Println("Default config.json created.")
	return nil
}

// LoadConfig loads configuration from a JSON file.
func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Use a temporary struct for JSON unmarshalling
	tempConfig := struct {
		BluetoothDeviceAddress string `json:"bluetooth_device_address"`
		CheckInterval          int    `json:"check_interval"` // Expect interval in seconds
		CheckRepeat            int    `json:"check_repeat"`
		DesktopEnv             string `json:"desktop_env"`
		Debug                  bool   `json:"debug"`
	}{}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&tempConfig)
	if err != nil {
		return nil, err
	}

	// Convert CheckInterval from seconds to time.Duration
	config := &Config{
		BluetoothDeviceAddress: tempConfig.BluetoothDeviceAddress,
		CheckInterval:          time.Duration(tempConfig.CheckInterval) * time.Second,
		CheckRepeat:            tempConfig.CheckRepeat,
		DesktopEnv:             tempConfig.DesktopEnv,
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

// Define RSSI thresholds for locking and unlocking
const (
	LockRSSI   = -70 // RSSI threshold for locking (when device is out of range)
	UnlockRSSI = -60 // RSSI threshold for unlocking (when device is in range)
)

// PingBluetoothDevice scans for the Bluetooth device, checking if it’s within a certain RSSI range.
func PingBluetoothDevice(deviceAddr string) (bool, error) {
	// Enable the Bluetooth adapter
	err := adapter.Enable()
	if err != nil {
		fmt.Println("Failed to enable Bluetooth adapter:", err)
		return false, err
	}

	// Channel to signal the RSSI state (in range or out of range)
	deviceInRange := make(chan bool, 1)

	// Start scanning in a separate goroutine
	go func() {
		adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			// Check if the discovered device matches the target address
			if result.Address.String() == deviceAddr {
				fmt.Println(result.RSSI)
				fmt.Printf("Found device with RSSI: %d dBm\n", result.RSSI)

				// Check RSSI against the thresholds
				if result.RSSI >= UnlockRSSI {
					// Device is close enough for unlocking
					deviceInRange <- true
				} else if result.RSSI <= LockRSSI {
					// Device is far enough to consider locking
					deviceInRange <- false
				}

				adapter.StopScan() // Stop scanning once the device is processed
			}
		})
	}()

	// Set a timeout for the scan (e.g., 5 seconds)
	timeout := time.After(5 * time.Second)

	// Wait for either a range determination or the timeout
	select {
	case inRange := <-deviceInRange:
		if inRange {
			fmt.Println("Device is in range.")
		} else {
			fmt.Println("Device is out of range.")
		}
		return inRange, nil
	case <-timeout:
		fmt.Println("Device not found within timeout.")
		adapter.StopScan() // Ensure scanning stops if timeout is reached
		return false, nil
	}
}

// MonitorBluetooth monitors the Bluetooth device connection and locks/unlocks based on range.
func MonitorBluetooth(config *Config) {
	mode := "locked" // Initial state

	for {
		// Check if the device is in range
		inRange, err := PingBluetoothDevice(config.BluetoothDeviceAddress)
		if err != nil {
			fmt.Println("Error during Bluetooth scan:", err)
			continue
		}

		if inRange && mode == "locked" {
			// Unlock if device is in range and was previously locked
			UnlockSystem(config.DesktopEnv)
			mode = "unlocked"
		} else if !inRange && mode == "unlocked" {
			// Lock if device is out of range and was previously unlocked
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
