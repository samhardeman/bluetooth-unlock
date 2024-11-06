package main

import (
	"bytes"
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Declare command-line flags.
var (
	BluetoothDeviceAddress string
	CheckInterval          time.Duration
	CheckRepeat            int
	LockRSSI               int
	UnlockRSSI             int
	DesktopEnv             string
	SessionTimeout         time.Duration
	Debug                  bool
)

// Default values for flags
const (
	defaultBluetoothDeviceAddress = "XX:XX:XX:XX:XX:XX"
	defaultCheckInterval          = 5 * time.Second
	defaultCheckRepeat            = 3
	defaultLockRSSI               = -14
	defaultUnlockRSSI             = -14
	defaultDesktopEnv             = "CINNAMON"
	defaultSessionTimeout         = 30 * time.Minute
	defaultDebug                  = true
)

// InitializeFlags initializes command-line flags and sets default values.
func InitializeFlags() {
	flag.StringVar(&BluetoothDeviceAddress, "bluetooth_device_address", defaultBluetoothDeviceAddress, "Bluetooth device address")
	flag.DurationVar(&CheckInterval, "check_interval", defaultCheckInterval, "Interval between checks")
	flag.IntVar(&CheckRepeat, "check_repeat", defaultCheckRepeat, "Number of times to check the device")
	flag.IntVar(&LockRSSI, "lock_rssi", defaultLockRSSI, "RSSI value to lock the system")
	flag.IntVar(&UnlockRSSI, "unlock_rssi", defaultUnlockRSSI, "RSSI value to unlock the system")
	flag.StringVar(&DesktopEnv, "desktop_env", defaultDesktopEnv, "Desktop environment (e.g., CINNAMON, GNOME, KDE)")
	flag.DurationVar(&SessionTimeout, "session_timeout", defaultSessionTimeout, "Session timeout duration")
	flag.BoolVar(&Debug, "debug", defaultDebug, "Enable debug mode")

	// Parse the flags
	flag.Parse()
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
func PingBluetoothDevice() (bool, error) {
	// Run `hcitool` to check RSSI
	cmd := exec.Command("hcitool", "rssi", BluetoothDeviceAddress)
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
		if rssi >= UnlockRSSI {
			return true, nil // Device is close enough for unlocking
		} else if rssi <= LockRSSI {
			return false, nil // Device is far enough to lock
		}
	}

	// If RSSI not found in output, assume device is out of range
	fmt.Println("Device not found or out of range.")
	return false, nil
}

// MonitorBluetooth monitors the Bluetooth device connection and locks/unlocks based on range.
func MonitorBluetooth() {
	mode := "locked"               // Initial state
	lastUnlockedTime := time.Now() // Track the last unlock time

	for {
		// Check if the device is in range using the configured RSSI thresholds
		inRange, err := PingBluetoothDevice()
		if err != nil {
			fmt.Println("Error during Bluetooth scan:", err)
			continue
		}

		currentTime := time.Now()

		// If device is in range and was previously locked, unlock it
		if inRange && mode == "locked" {
			UnlockSystem(DesktopEnv)
			lastUnlockedTime = currentTime // Update the last unlocked time
			mode = "unlocked"
		} else if !inRange && mode == "unlocked" {
			// If device is out of range and was previously unlocked, lock it
			LockSystem(DesktopEnv)
			mode = "locked"
		}

		// If the device is disconnected and the session is unlocked, lock the system
		if !inRange && mode == "unlocked" {
			// Lock system if device is disconnected
			LockSystem(DesktopEnv)
			mode = "locked"
		}

		// Check for session timeout
		if mode == "unlocked" && currentTime.Sub(lastUnlockedTime) > SessionTimeout {
			fmt.Println("Session timeout reached. Locking system.")
			LockSystem(DesktopEnv)
			mode = "locked"
		}

		// Wait before the next check
		time.Sleep(CheckInterval)
	}
}

func main() {
	// Initialize command-line flags
	InitializeFlags()

	// Print the parsed config values
	fmt.Println("Bluetooth Unlock is now active!")
	fmt.Printf("Desktop Environment: %s\n", DesktopEnv)
	fmt.Printf("Bluetooth Device Address: %s\n", BluetoothDeviceAddress)

	// Monitor Bluetooth connection and manage lock/unlock states
	MonitorBluetooth()
}
