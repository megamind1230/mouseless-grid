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
	Version    bool   `short:"v" long:"version" description:"Show version"`
	Debug      bool   `short:"d" long:"debug" description:"Verbose debug output"`
	Config     string `short:"c" long:"config" description:"Path to config file"`
	ListDev    bool   `short:"l" long:"list-devices" description:"List keyboard devices"`
	GenConf    bool   `long:"gen-config" description:"Generate default config file"`
	SavDebug   bool   `long:"save-debug" description:"Save rendered overlay to PNG"`
	Check      bool   `long:"check" description:"Create overlay, log window ID, sleep 10s (for xprop diagnostics)"`
	GridLayout string `long:"grid-layout" default:"num" choice:"num" choice:"home" description:"Grid layout: num (1-9) or home (asdfghjkl)"`
}

const version = "0.2.0"

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	if opts.Version {
		fmt.Println(version)
		os.Exit(0)
	}

	log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: "15:04:05.000"})
	if opts.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	cfg, err := config.Load(opts.Config)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Debug = opts.Debug
	cfg.ConfigPath = opts.Config
	if opts.GridLayout != "num" {
		cfg.GridLayout = opts.GridLayout
	}

	if err := config.EnsureLogDir(cfg); err != nil {
		log.Warnf("Failed to create log dir: %v", err)
	}

	logFile := setupFileLogging(cfg)
	if logFile != nil {
		defer logFile.Close()
	}

	if opts.GenConf {
		if err := config.Save(cfg, opts.Config); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		fmt.Printf("Config written to %s\n", opts.Config)
		os.Exit(0)
	}

	if opts.ListDev {
		printKeyboardDevices()
		os.Exit(0)
	}

	if opts.Check {
		runCheck(cfg)
		os.Exit(0)
	}

	log.Info("mouseless-grid starting")

	if isWayland() {
		log.Info("Running on Wayland")
		runWayland(cfg)
	} else {
		log.Info("Running on X11")
		runX11(cfg)
	}
}

func isWayland() bool {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return true
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return false
}

func runX11(cfg *config.Config) {
	ov, err := overlay.New()
	if err != nil {
		log.Fatalf("Failed to create overlay: %v", err)
	}
	defer ov.Close()

	sW := int(ov.Width())
	sH := int(ov.Height())
	log.Infof("Screen: %dx%d", sW, sH)
	ov.SetOpacity(cfg.Opacity)

	g := grid.New(cfg, sW, sH)
	log.Infof("Grid: %s", g)

	vmouse, err := mouse.NewX11(ov.Conn())
	if err != nil {
		log.Fatalf("Failed to create virtual mouse: %v", err)
	}
	defer vmouse.Close()

	keyChan := make(chan keyboard.Event, 100)
	keyboards, err := keyboard.ListKeyboardDevices()
	if err != nil {
		log.Fatalf("Failed to list keyboard devices: %v", err)
	}

	if len(keyboards) == 0 {
		log.Fatal("No keyboard devices found")
	}

	var kbs []*keyboard.Device
	for _, dev := range keyboards {
		kd := keyboard.NewKeyboardDevice(dev, keyChan)
		if err := kd.GrabDevice(); err != nil {
			log.Warnf("Failed to grab %s: %v", dev.Fn, err)
			continue
		}
		kbs = append(kbs, kd)
		log.Infof("Grabbed keyboard: %s", dev.Name)
	}
	defer func() {
		for _, kb := range kbs {
			kb.Close()
		}
	}()

	var debugPath string
	if opts.SavDebug {
		debugPath = filepath.Join(cfg.LogPath, "debug-overlay.png")
	}
	bgColor := config.ParseColor(cfg.BgColor)
	highlightColor := config.ParseColor(cfg.HighlightColor)
	renderOverlay(ov, g, bgColor, highlightColor, 1.0, debugPath)

	go handleX11Events(ov, g, bgColor, highlightColor, 1.0, debugPath)

	log.Info("Ready. Type 1-9 to zoom, Enter to confirm, i/o to click, Esc to exit.")
	for ev := range keyChan {
		if !ev.IsPress {
			continue
		}

		alias, _ := getAliasForCode(ev.Code)

		if alias == "esc" {
			log.Info("Escape pressed, exiting")
			return
		}

		switch alias {
		case "i":
			ov.Hide()
			time.Sleep(50 * time.Millisecond)
			log.Infof("Left click at (%d, %d)", g.ClickX(), g.ClickY())
			if err := vmouse.ClickAt(int32(g.ClickX()), int32(g.ClickY())); err != nil {
				log.Errorf("Click failed: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			log.Info("Done")
			return
		case "o":
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

		if g.HandleKey(ev) {
			if g.State() == grid.StateReady {
				if err := vmouse.MoveTo(int32(g.ClickX()), int32(g.ClickY())); err != nil {
					log.Warnf("MoveTo failed: %v", err)
				}
			}
			renderOverlay(ov, g, bgColor, highlightColor, 1.0, debugPath)
		}
	}
}

func runWayland(cfg *config.Config) {
	keyChan := make(chan keyboard.Event, 100)
	ov, err := overlay.NewWayland(keyChan)
	if err != nil {
		log.Fatalf("Failed to create overlay: %v", err)
	}
	defer ov.Close()

	sW := int(ov.Width())
	sH := int(ov.Height())
	log.Infof("Screen: %dx%d", sW, sH)

	g := grid.New(cfg, sW, sH)
	log.Infof("Grid: %s", g)

	vmouse, err := mouse.NewWayland()
	if err != nil {
		log.Fatalf("Failed to create virtual mouse: %v", err)
	}
	defer vmouse.Close()

	var debugPath string
	if opts.SavDebug {
		debugPath = filepath.Join(cfg.LogPath, "debug-overlay.png")
	}
	bgColor := config.ParseColor(cfg.BgColor)
	highlightColor := config.ParseColor(cfg.HighlightColor)
	renderOverlay(ov, g, bgColor, highlightColor, cfg.Opacity, debugPath)

	log.Info("Ready. Type 1-9 to zoom, Enter to confirm, i/o to click, Esc to exit.")
	for ev := range keyChan {
		if !ev.IsPress {
			continue
		}

		alias, _ := getAliasForCode(ev.Code)

		if alias == "esc" {
			log.Info("Escape pressed, exiting")
			return
		}

		switch alias {
		case "i":
			ov.Hide()
			time.Sleep(50 * time.Millisecond)
			log.Infof("Left click at (%d, %d)", g.ClickX(), g.ClickY())
			if err := vmouse.ClickAt(int32(g.ClickX()), int32(g.ClickY())); err != nil {
				log.Errorf("Click failed: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			log.Info("Done")
			return
		case "o":
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

		if g.HandleKey(ev) {
			if g.State() == grid.StateReady {
				if err := vmouse.MoveTo(int32(g.ClickX()), int32(g.ClickY())); err != nil {
					log.Warnf("MoveTo failed: %v", err)
				}
			}
			renderOverlay(ov, g, bgColor, highlightColor, cfg.Opacity, debugPath)
		}
	}
}

func renderOverlay(ov overlay.Window, g *grid.Grid, bgColor, highlightColor color.Color, opacity float64, debugPath string) {
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

func runCheck(cfg *config.Config) {
	if isWayland() {
		ov, err := overlay.NewWayland(nil)
		if err != nil {
			log.Fatalf("Failed to create overlay: %v", err)
		}
		defer ov.Close()
		sW := int(ov.Width())
		sH := int(ov.Height())
		g := grid.New(cfg, sW, sH)

		bgColor := config.ParseColor(cfg.BgColor)
		highlightColor := config.ParseColor(cfg.HighlightColor)
		img := g.Render(bgColor, nil, highlightColor, cfg.Opacity)
		if err := ov.Render(img); err != nil {
			log.Warnf("Render failed: %v", err)
		}

		log.Info("=== CHECK MODE === (Wayland)")
		log.Infof("Sleeping 10s from Ctrl+C to exit early")
		time.Sleep(10 * time.Second)
		return
	}

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
	log.Infof("Sleeping 10s from Ctrl+C to exit early")
	time.Sleep(10 * time.Second)
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

func setupFileLogging(cfg *config.Config) *os.File {
	logName := fmt.Sprintf("mouseless-grid_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(cfg.LogPath, logName)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Warnf("Failed to open log file %s: %v", logPath, err)
		return nil
	}

	multiWriter := io.MultiWriter(os.Stdout, f)
	log.SetOutput(multiWriter)

	log.Infof("Logging to %s", logPath)
	return f
}
