package keyboard

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"mouseless-grid/config"

	evdev "github.com/gvalkov/golang-evdev"
	log "github.com/sirupsen/logrus"
)

// Event represents a keyboard press or release
type Event struct {
	Code    uint16
	IsPress bool
	Time    time.Time
}

// Device wraps an evdev keyboard device with a read loop
type Device struct {
	device    *evdev.InputDevice
	eventChan chan<- Event
	done      chan struct{}
}

// NewKeyboardDevice creates a new keyboard device wrapper
func NewKeyboardDevice(device *evdev.InputDevice, eventChan chan<- Event) *Device {
	return &Device{
		device:    device,
		eventChan: eventChan,
		done:      make(chan struct{}),
	}
}

// GrabDevice exclusively grabs the keyboard and starts the read loop
func (d *Device) GrabDevice() error {
	err := d.device.Grab()
	if err != nil {
		return err
	}
	log.Debugf("Grabbed device: %s (%s)", d.device.Fn, d.device.Name)

	go d.readLoop()
	return nil
}

// readLoop reads evdev events in a goroutine, sends press/release to channel
func (d *Device) readLoop() {
	for {
		events, err := d.device.Read()
		if err != nil {
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				log.Warnf("Failed to read keyboard %s: %v", d.device.Fn, err)
			}
			return
		}
		for _, event := range events {
			if event.Type != evdev.EV_KEY {
				continue
			}
			// only care about press (1) and release (0)
			if event.Value != 0 && event.Value != 1 {
				continue
			}

			codeAlias, exists := config.GetKeyAlias(event.Code)
			if !exists {
				codeAlias = "?"
			}
			if event.Value == 1 {
				log.Infof("Pressed:  %s (%d)", codeAlias, event.Code)
			} else {
				log.Debugf("Released: %s (%d)", codeAlias, event.Code)
			}

			select {
			case d.eventChan <- Event{
				Code:    event.Code,
				IsPress: event.Value == 1,
				Time:    time.Now(),
			}:
			case <-d.done:
				return
			}
		}
	}
}

func (d *Device) Close() error {
	close(d.done)
	_ = d.device.File.Close()
	return nil
}

func (d *Device) Path() string { return d.device.Fn }
func (d *Device) Name() string { return d.device.Name }
func (d *Device) String() string {
	return fmt.Sprintf("%s (%s)", d.device.Fn, d.device.Name)
}

// ListKeyboardDevices enumerates /dev/input/event* and returns keyboard devices
func ListKeyboardDevices() ([]*evdev.InputDevice, error) {
	allDevices, err := evdev.ListInputDevices("/dev/input/event*")
	if err != nil {
		return nil, err
	}

	var keyboards []*evdev.InputDevice
	for _, dev := range allDevices {
		if isKeyboardDevice(dev) {
			keyboards = append(keyboards, dev)
		}
	}
	return keyboards, nil
}

// isKeyboardDevice checks if a device has KEY_A or KEY_KP1 (standard keyboard check)
func isKeyboardDevice(dev *evdev.InputDevice) bool {
	for capType, codes := range dev.Capabilities {
		if capType.Type == evdev.EV_KEY {
			for _, code := range codes {
				if code.Code == evdev.KEY_A || code.Code == evdev.KEY_KP1 {
					return true
				}
			}
		}
	}
	return false
}

// OpenDevice tries to open a device path with retries (for udev permission races)
func OpenDevice(path string) (*evdev.InputDevice, error) {
	var device *evdev.InputDevice
	var err error
	for range 3 {
		device, err = evdev.Open(path)
		if err == nil {
			return device, nil
		}
		time.Sleep(1000 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to open %s after 3 retries: %w", path, err)
}

// WatchDevices watches /dev/input for keyboard hot-plug events
// baka: this returns channels for created/removed device paths
func WatchDevices(keyboardNames []string) (created chan string, removed chan string, err error) {
	// not implementing fsnotify here — will add in Phase 7 integration
	// for now, just return nil channels
	created = make(chan string, 10)
	removed = make(chan string, 10)
	_ = filepath.Clean // baka: ensuring filepath is imported for future use
	return
}
