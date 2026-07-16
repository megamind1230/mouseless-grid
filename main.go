package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mouseless-grid/config"
	"mouseless-grid/grid"
	"mouseless-grid/keyboard"
	"mouseless-grid/mouse"
	"mouseless-grid/overlay"

	"github.com/jezek/xgb/xproto"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

var opts struct {
	Version  bool   `short:"v" long:"version" description:"Show version"`
	Debug    bool   `short:"d" long:"debug" description:"Verbose debug output"`
	Config   string `short:"c" long:"config" description:"Path to config file"`
	ListDev  bool   `short:"l" long:"list-devices" description:"List keyboard devices"`
	GenConf  bool   `long:"gen-config" description:"Generate default config file"`
	SavDebug bool   `long:"save-debug" description:"Save rendered overlay to PNG"`
	Check    bool   `long:"check" description:"Create overlay, log window ID, sleep 10s (for xprop diagnostics)"`
}

const version = "0.1.0"

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	if opts.Version {
		fmt.Println(version)
		os.Exit(0)
	}

	// init logging
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: "15:04:05.000"})
	if opts.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// load config
	cfg, err := config.Load(opts.Config)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Debug = opts.Debug
	cfg.ConfigPath = opts.Config

	if err := config.EnsureLogDir(cfg); err != nil {
		log.Warnf("Failed to create log dir: %v", err)
	}

	// setup file logging
	logFile := setupFileLogging(cfg)
	if logFile != nil {
		defer logFile.Close()
	}

	// generate config and exit
	if opts.GenConf {
		if err := config.Save(cfg, opts.Config); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		fmt.Printf("Config written to %s\n", opts.Config)
		os.Exit(0)
	}

	// list devices and exit
	if opts.ListDev {
		printKeyboardDevices()
		os.Exit(0)
	}

	// check mode: create overlay, render, log WID, sleep for xprop diagnostics
	if opts.Check {
		runCheck(cfg)
		os.Exit(0)
	}

	log.Info("mouseless-grid starting")
	run(cfg)
}

func run(cfg *config.Config) {
	// 1. create overlay
	ov, err := overlay.New()
	if err != nil {
		log.Fatalf("Failed to create overlay: %v", err)
	}
	defer ov.Close()

	sW := int(ov.Width())
	sH := int(ov.Height())
	log.Infof("Screen: %dx%d", sW, sH)
	ov.SetOpacity(cfg.Opacity)

	// 2. create grid
	g := grid.New(cfg, sW, sH)
	log.Infof("Grid: %s", g)

	// 3. create virtual mouse via XTest
	vmouse, err := mouse.New(ov.Conn())
	if err != nil {
		log.Fatalf("Failed to create virtual mouse: %v", err)
	}
	defer vmouse.Close()

	// 4. grab keyboard devices
	keyChan := make(chan keyboard.Event, 100)
	keyboards, err := keyboard.ListKeyboardDevices()
	if err != nil {
		log.Fatalf("Failed to list keyboard devices: %v", err)
	}

	if len(keyboards) == 0 {
		log.Fatal("No keyboard devices found")
	}

	for _, dev := range keyboards {
		kd := keyboard.NewKeyboardDevice(dev, keyChan)
		if err := kd.GrabDevice(); err != nil {
			log.Warnf("Failed to grab %s: %v", dev.Fn, err)
			continue
		}
		log.Infof("Grabbed keyboard: %s", dev.Name)
	}

	// 5. initial render
	var debugPath string
	if opts.SavDebug {
		debugPath = filepath.Join(cfg.LogPath, "debug-overlay.png")
	}
	bgColor := config.ParseColor(cfg.BgColor)
	highlightColor := config.ParseColor(cfg.HighlightColor)
	renderOverlay(ov, g, bgColor, highlightColor, 1.0, debugPath)

	// 6. handle X11 events in background (expose, etc.)
	go handleX11Events(ov, g, bgColor, highlightColor, 1.0, debugPath)

	// 7. main event loop: keyboard → grid → render/click
	log.Info("Ready. Type 1-9 to zoom, Enter to confirm, LCtrl/LAlt to click, Esc to exit.")
	for ev := range keyChan {
		if !ev.IsPress {
			continue
		}

		alias, _ := getAliasForCode(ev.Code)

		// escape always exits
		if alias == "esc" {
			log.Info("Escape pressed, exiting")
			return
		}

		// click choice in Ready state (handled here, not by grid)
		if g.State() == grid.StateReady {
			switch alias {
			case "leftctrl":
				ov.Hide()
				time.Sleep(50 * time.Millisecond)
				log.Infof("Left click at (%d, %d)", g.ClickX(), g.ClickY())
				if err := vmouse.ClickAt(int32(g.ClickX()), int32(g.ClickY())); err != nil {
					log.Errorf("Click failed: %v", err)
				}
				time.Sleep(100 * time.Millisecond)
				log.Info("Done")
				return
			case "leftalt":
				ov.Hide()
				time.Sleep(50 * time.Millisecond)
				log.Infof("Right click at (%d, %d)", g.ClickX(), g.ClickY())
				if err := vmouse.RightClickAt(int32(g.ClickX()), int32(g.ClickY())); err != nil {
					log.Errorf("Right click failed: %v", err)
				}
				time.Sleep(100 * time.Millisecond)
				log.Info("Done")
				return
			}
		}

		if g.HandleKey(ev) {
			// when entering Ready state, move cursor to show position
			if g.State() == grid.StateReady {
				if err := vmouse.MoveTo(int32(g.ClickX()), int32(g.ClickY())); err != nil {
					log.Warnf("MoveTo failed: %v", err)
				}
			}

			// re-render on state change
			renderOverlay(ov, g, bgColor, highlightColor, 1.0, debugPath)
		}
	}
}

func renderOverlay(ov *overlay.Overlay, g *grid.Grid, bgColor, highlightColor color.Color, opacity float64, debugPath string) {
	img := g.Render(bgColor, nil, highlightColor, opacity)
	if debugPath != "" {
		if err := overlay.SaveDebugImage(img, debugPath); err != nil {
			log.Warnf("Failed to save debug image: %v", err)
		} else {
			log.Infof("Saved debug overlay to %s", debugPath)
		}
	}
	if err := ov.Render(img); err != nil {
		log.Warnf("Render failed: %v", err)
	}
}

func handleX11Events(ov *overlay.Overlay, g *grid.Grid, bgColor, highlightColor color.Color, opacity float64, debugPath string) {
	conn := ov.Conn()
	for {
		ev, xerr := conn.WaitForEvent()
		if ev == nil && xerr == nil {
			return
		}
		switch e := ev.(type) {
		case xproto.ExposeEvent:
			if e.Count == 0 {
				img := g.Render(bgColor, nil, highlightColor, opacity)
				if debugPath != "" {
					overlay.SaveDebugImage(img, debugPath)
				}
				ov.Render(img)
			}
		}
	}
}

func runCheck(cfg *config.Config) {
	ov, err := overlay.New()
	if err != nil {
		log.Fatalf("Failed to create overlay: %v", err)
	}
	defer ov.Close()

	sW := int(ov.Width())
	sH := int(ov.Height())
	g := grid.New(cfg, sW, sH)
	ov.SetOpacity(cfg.Opacity)

	bgColor := config.ParseColor(cfg.BgColor)
	highlightColor := config.ParseColor(cfg.HighlightColor)
	img := g.Render(bgColor, nil, highlightColor, 1.0)
	if err := ov.Render(img); err != nil {
		log.Warnf("Render failed: %v", err)
	}

	wid := ov.Window()
	log.Infof("=== CHECK MODE === Window ID: %d", wid)
	log.Infof("Run: xprop -id %d", wid)
	log.Infof("Sleeping 10s — press Ctrl+C to exit early")
	time.Sleep(10 * time.Second)
}

// getAliasForCode is a local alias lookup (duplicates keydefs for simplicity)
func getAliasForCode(code uint16) (string, bool) {
	aliases := map[uint16]string{
		1: "esc", 14: "backspace", 28: "enter", 57: "space",
		29: "leftctrl", 56: "leftalt", 97: "rightctrl", 100: "rightalt",
		2: "k1", 3: "k2", 4: "k3", 5: "k4", 6: "k5",
		7: "k6", 8: "k7", 9: "k8", 10: "k9", 11: "k0",
		36: "j", 37: "k", 38: "l",
		22: "u", 23: "i", 24: "o",
		50: "m", 51: "comma", 52: "dot",
		39: "semicolon", 40: "apostrophe",
		30: "a", 31: "s", 32: "d", 33: "f", 34: "g",
		35: "h", 21: "y",
		49: "n", 48: "b", 47: "v", 46: "c", 45: "x", 44: "z",
		16: "q", 17: "w", 18: "e", 19: "r", 20: "t",
	}
	a, ok := aliases[code]
	return a, ok
}

func printKeyboardDevices() {
	keyboards, err := keyboard.ListKeyboardDevices()
	if err != nil {
		log.Fatalf("Failed to list devices: %v", err)
	}
	if len(keyboards) == 0 {
		fmt.Println("No keyboard devices found")
		return
	}
	sort.Slice(keyboards, func(i, j int) bool {
		return keyboards[i].Name < keyboards[j].Name
	})
	headers := []string{"Name", "Device"}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	type row struct{ name, path string }
	var rows []row
	for _, dev := range keyboards {
		rows = append(rows, row{dev.Name, dev.Fn})
		if len(dev.Name) > widths[0] {
			widths[0] = len(dev.Name)
		}
		if len(dev.Fn) > widths[1] {
			widths[1] = len(dev.Fn)
		}
	}
	printRow := func(vals ...string) {
		for i, v := range vals {
			fmt.Print(v)
			if i < len(vals)-1 {
				fmt.Print(strings.Repeat(" ", 2+widths[i]-len(v)))
			}
		}
		fmt.Println()
	}
	printRow(headers...)
	for _, r := range rows {
		printRow(r.name, r.path)
	}
}

// setupFileLogging creates a log file and configures multi-writer output
func setupFileLogging(cfg *config.Config) *os.File {
	logName := fmt.Sprintf("mouseless-grid_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(cfg.LogPath, logName)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Warnf("Failed to open log file %s: %v", logPath, err)
		return nil
	}

	// stdout + file, both at configured level
	multiWriter := io.MultiWriter(os.Stdout, f)
	log.SetOutput(multiWriter)

	log.Infof("Logging to %s", logPath)
	return f
}
