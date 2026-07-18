package overlay

import (
	"fmt"
	"image"
	"os"
	"syscall"
	"time"

	"mouseless-grid/keyboard"
	"mouseless-grid/wlr"

	wlos "github.com/neurlang/wayland/os"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	log "github.com/sirupsen/logrus"
)

type waylandState struct {
	display    *wl.Display
	compositor *wl.Compositor
	shm        *wl.Shm
	seat       *wl.Seat
	keyboard   *wl.Keyboard
	output     *wl.Output
	layerShell *wlr.ZwlrLayerShellV1
	virtPtrMgr *wlr.ZwlrVirtualPointerManagerV1
	width      uint32
	height     uint32
	ready      bool
	keyChan    chan<- keyboard.Event
}

var wlState waylandState

func WlState() *waylandState {
	return &wlState
}

func (s *waylandState) Display() *wl.Display                        { return s.display }
func (s *waylandState) Compositor() *wl.Compositor                  { return s.compositor }
func (s *waylandState) Shm() *wl.Shm                                { return s.shm }
func (s *waylandState) Seat() *wl.Seat                              { return s.seat }
func (s *waylandState) Output() *wl.Output                          { return s.output }
func (s *waylandState) VirtPtrMgr() *wlr.ZwlrVirtualPointerManagerV1 { return s.virtPtrMgr }
func (s *waylandState) ScreenWidth() uint32                         { return s.width }
func (s *waylandState) ScreenHeight() uint32                        { return s.height }

type waylandRegistryHandler struct{}

func (h *waylandRegistryHandler) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	goFace := ev.Interface
	reg, _ := wlState.display.GetRegistry()
	ctx, _ := wl.GetUserData[wl.Context](reg)
	switch goFace {
	case "wl_compositor":
		wlState.compositor = wlclient.RegistryBindCompositorInterface(reg, ev.Name, ev.Version)
		log.Debugf("Wayland: bound wl_compositor v%d", ev.Version)
	case "wl_shm":
		wlState.shm = wlclient.RegistryBindShmInterface(reg, ev.Name, ev.Version)
		log.Debugf("Wayland: bound wl_shm v%d", ev.Version)
	case "wl_seat":
		v := ev.Version
		if v > 4 {
			v = 4
		}
		if v < 4 {
			log.Warnf("Wayland: seat version %d < 4, keyboard input unavailable", v)
		}
		wlState.seat = wlclient.RegistryBindSeatInterface(reg, ev.Name, v)
		log.Debugf("Wayland: bound wl_seat v%d", ev.Version)
	case "wl_output":
		wlState.output = wlclient.RegistryBindOutputInterface(reg, ev.Name, 1)
		wlclient.OutputAddListener(wlState.output, &outputHandler{})
		log.Debugf("Wayland: bound wl_output v%d", ev.Version)
	case "zwlr_layer_shell_v1":
		wlState.layerShell = wlr.NewZwlrLayerShellV1(ctx)
		reg.Bind(ev.Name, "zwlr_layer_shell_v1", 1, wlState.layerShell)
		log.Debugf("Wayland: bound zwlr_layer_shell_v1")
	case "zwlr_virtual_pointer_manager_v1":
		wlState.virtPtrMgr = wlr.NewZwlrVirtualPointerManagerV1(ctx)
		reg.Bind(ev.Name, "zwlr_virtual_pointer_manager_v1", 1, wlState.virtPtrMgr)
		log.Debugf("Wayland: bound zwlr_virtual_pointer_manager_v1")
	}
}

func (h *waylandRegistryHandler) HandleRegistryGlobalRemove(ev wl.RegistryGlobalRemoveEvent) {}

type wlKeyboardHandler struct{}

func (h *wlKeyboardHandler) HandleKeyboardKeymap(ev wl.KeyboardKeymapEvent) {
	if ev.Fd != 0 {
		syscall.Close(int(ev.Fd))
	}
}
func (h *wlKeyboardHandler) HandleKeyboardEnter(wl.KeyboardEnterEvent) {}
func (h *wlKeyboardHandler) HandleKeyboardLeave(wl.KeyboardLeaveEvent) {}
func (h *wlKeyboardHandler) HandleKeyboardKey(ev wl.KeyboardKeyEvent) {
	wlState.keyChan <- keyboard.Event{
		Code:    uint16(ev.Key),
		IsPress: ev.State == wl.KeyboardKeyStatePressed,
		Time:    time.Now(),
	}
}
func (h *wlKeyboardHandler) HandleKeyboardModifiers(wl.KeyboardModifiersEvent) {}
func (h *wlKeyboardHandler) HandleKeyboardRepeatInfo(wl.KeyboardRepeatInfoEvent) {}

type outputHandler struct{}

func (o *outputHandler) HandleOutputGeometry(ev wl.OutputGeometryEvent) {}
func (o *outputHandler) HandleOutputMode(ev wl.OutputModeEvent) {
	if ev.Flags&wl.OutputModeCurrent != 0 {
		wlState.width = uint32(ev.Width)
		wlState.height = uint32(ev.Height)
		log.Debugf("Wayland: output mode %dx%d @ %d mHz", ev.Width, ev.Height, ev.Refresh)
	}
}
func (o *outputHandler) HandleOutputDone(ev wl.OutputDoneEvent)     {}
func (o *outputHandler) HandleOutputScale(ev wl.OutputScaleEvent)   {
	log.Debugf("Wayland: output scale %d", ev.Factor)
}

type layerSurfaceHandler struct {
	surface *wlr.ZwlrLayerSurfaceV1
}

func (h *layerSurfaceHandler) HandleZwlrLayerSurfaceV1Configure(ev wlr.ZwlrLayerSurfaceV1ConfigureEvent) {
	h.surface.AckConfigure(ev.Serial)
	// ponytail: ignore compositor's suggested size — we use full output dimensions from output mode
}

func (h *layerSurfaceHandler) HandleZwlrLayerSurfaceV1Closed(wlr.ZwlrLayerSurfaceV1ClosedEvent) {
	log.Warn("Wayland: layer surface closed by compositor")
}

func waylandSetup() error {
	if wlState.ready {
		return nil
	}

	d, err := wlclient.DisplayConnect(nil)
	if err != nil {
		return fmt.Errorf("connect wayland: %w", err)
	}
	wlState.display = d

	reg, err := d.GetRegistry()
	if err != nil {
		return fmt.Errorf("get registry: %w", err)
	}

	wlclient.RegistryAddListener(reg, &waylandRegistryHandler{})

	if err := wlclient.DisplayRoundtrip(d); err != nil {
		return err
	}
	if err := wlclient.DisplayRoundtrip(d); err != nil {
		return err
	}

	if wlState.compositor == nil {
		return fmt.Errorf("no wl_compositor")
	}
	if wlState.shm == nil {
		return fmt.Errorf("no wl_shm")
	}
	if wlState.output == nil {
		return fmt.Errorf("no wl_output")
	}
	if wlState.layerShell == nil {
		return fmt.Errorf("no zwlr_layer_shell_v1")
	}
	if wlState.virtPtrMgr == nil {
		return fmt.Errorf("no zwlr_virtual_pointer_manager_v1")
	}
	if wlState.seat == nil {
		return fmt.Errorf("no wl_seat")
	}
	if wlState.width == 0 || wlState.height == 0 {
		wlState.width, wlState.height = 1920, 1080
		log.Warn("Wayland: no output mode received, assuming 1920x1080")
	}

	wlState.ready = true
	log.Infof("Wayland: display setup complete (%dx%d)", wlState.width, wlState.height)
	return nil
}

func dispatchWaylandEvents() {
	for {
		if err := wlclient.DisplayDispatch(wlState.display); err != nil {
			if err == wl.ErrContextRunProxyNil {
				continue
			}
			log.Warnf("Wayland dispatch: %v", err)
			return
		}
	}
}

type WaylandOverlay struct {
	width, height uint32
	surface       *wl.Surface
	layerSurface  *wlr.ZwlrLayerSurfaceV1
}

func NewWayland(keyChan chan keyboard.Event) (*WaylandOverlay, error) {
	if err := waylandSetup(); err != nil {
		return nil, err
	}

	wlState.keyChan = keyChan

	if keyChan != nil && wlState.seat != nil {
		kb, err := wlState.seat.GetKeyboard()
		if err == nil {
			kb.AddKeymapHandler(&wlKeyboardHandler{})
			kb.AddEnterHandler(&wlKeyboardHandler{})
			kb.AddLeaveHandler(&wlKeyboardHandler{})
			kb.AddKeyHandler(&wlKeyboardHandler{})
			kb.AddModifiersHandler(&wlKeyboardHandler{})
			kb.AddRepeatInfoHandler(&wlKeyboardHandler{})
			log.Debug("Wayland: keyboard listener active")
		} else {
			log.Warnf("Wayland: failed to get keyboard: %v", err)
		}
	}

	w, h := wlState.width, wlState.height

	surf, err := wlState.compositor.CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}

	layerSurf, err := wlState.layerShell.GetLayerSurface(
		surf, wlState.output, wlr.ZwlrLayerShellV1LayerOverlay, "mouseless-grid")
	if err != nil {
		return nil, fmt.Errorf("get layer surface: %w", err)
	}
	log.Debug("Wayland: got layer surface")

	layerSurf.SetSize(w, h)
	layerSurf.SetAnchor(wlr.ZwlrLayerSurfaceV1AnchorTop | wlr.ZwlrLayerSurfaceV1AnchorBottom |
		wlr.ZwlrLayerSurfaceV1AnchorLeft | wlr.ZwlrLayerSurfaceV1AnchorRight)
	layerSurf.SetExclusiveZone(-1)
	layerSurf.SetKeyboardInteractivity(wlr.ZwlrLayerSurfaceV1KeyboardInteractivityExclusive)

	surf.SetInputRegion(nil)
	log.Debug("Wayland: configured layer surface (fullscreen, click-through)")

	layerSurf.AddConfigureHandler(&layerSurfaceHandler{surface: layerSurf})
	layerSurf.AddClosedHandler(&layerSurfaceHandler{surface: layerSurf})

	surf.Commit()
	wlclient.DisplayRoundtrip(wlState.display)

	go dispatchWaylandEvents()

	return &WaylandOverlay{
		width: w, height: h,
		surface:      surf,
		layerSurface: layerSurf,
	}, nil
}

func (o *WaylandOverlay) Render(img *image.NRGBA) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	f, data, err := createSHM(w, h)
	if err != nil {
		return err
	}
	defer f.Close()

	copyImageToSHM(img, data)

	size := w * h * 4
	pool, err := wlState.shm.CreatePool(f.Fd(), int32(size))
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}

	buf, err := pool.CreateBuffer(0, int32(w), int32(h), int32(w*4), wl.ShmFormatArgb8888)
	if err != nil {
		pool.Destroy()
		return fmt.Errorf("create buffer: %w", err)
	}

	pool.Destroy()
	o.surface.Attach(buf, 0, 0)
	o.surface.Damage(0, 0, int32(w), int32(h))
	o.surface.Commit()
	log.Debugf("Wayland: rendered %dx%d buffer to surface", w, h)
	return nil
}

func createSHM(width, height int) (*os.File, []byte, error) {
	stride := width * 4
	size := stride * height

	f, err := wlos.CreateAnonymousFile(int64(size))
	if err != nil {
		return nil, nil, err
	}

	data, err := wlos.Mmap(int(f.Fd()), 0, size, wlos.ProtRead|wlos.ProtWrite, wlos.MapShared)
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	// ponytail: fd closed by caller after CreatePool, mmap stays valid
	return f, data, nil
}

func (o *WaylandOverlay) Width() uint16  { return uint16(o.width) }
func (o *WaylandOverlay) Height() uint16 { return uint16(o.height) }

func (o *WaylandOverlay) SetOpacity(float64) {}

func (o *WaylandOverlay) Hide() {
	log.Debug("Wayland: hiding overlay")
	o.surface.Attach(nil, 0, 0)
	o.surface.Commit()
}

func (o *WaylandOverlay) Close() {
	log.Debug("Wayland: closing overlay")
	o.layerSurface.Destroy()
	o.surface.Destroy()
	if wlState.keyboard != nil {
		wlState.keyboard.Release()
		wlState.keyboard = nil
	}
}

func copyImageToSHM(img *image.NRGBA, data []byte) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.NRGBAAt(x, y).RGBA()
			i := (y*w + x) * 4
			data[i+0] = byte(b >> 8)
			data[i+1] = byte(g >> 8)
			data[i+2] = byte(r >> 8)
			data[i+3] = byte(a >> 8)
		}
	}
}
