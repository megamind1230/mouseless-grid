package overlay

import (
	"fmt"
	"image"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/shape"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
	log "github.com/sirupsen/logrus"
)

type Overlay struct {
	conn   *xgb.Conn
	win    xproto.Window
	cmap   xproto.Colormap
	gc     xproto.Gcontext
	screen xproto.ScreenInfo
	width  uint16
	height uint16
}

func New() (*Overlay, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X display: %w", err)
	}

	screen := xproto.Setup(conn).Roots[0]
	sW := screen.WidthInPixels
	sH := screen.HeightInPixels
	log.Debugf("Screen: %dx%d", sW, sH)

	visual, depth := findARGBVisual(conn, screen)
	if visual == 0 {
		conn.Close()
		return nil, fmt.Errorf("no 32-bit ARGB visual found")
	}
	log.Debugf("ARGB visual: id=%d depth=%d", visual, depth)

	cmap, err := xproto.NewColormapId(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	xproto.CreateColormap(conn, xproto.ColormapAllocNone, cmap, screen.Root, visual)

	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	eventMask := xproto.EventMaskExposure |
		xproto.EventMaskStructureNotify |
		xproto.EventMaskKeyPress |
		xproto.EventMaskKeyRelease

	if err := xproto.CreateWindowChecked(conn,
		depth, wid, screen.Root,
		0, 0, sW, sH, 0,
		xproto.WindowClassInputOutput, visual,
		xproto.CwBackPixmap|xproto.CwBorderPixel|xproto.CwEventMask|xproto.CwColormap,
		[]uint32{0, 0, uint32(eventMask), uint32(cmap)}).Check(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("CreateWindow failed: %w", err)
	}

	setWindowType(conn, wid)
	setAlwaysOnTopProperty(conn, wid)
	setSticky(conn, wid)

	shape.Init(conn)
	xfixes.Init(conn)
	shape.QueryVersion(conn)
	makeClickThrough(conn, wid)

	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	xproto.CreateGC(conn, gc, xproto.Drawable(wid), 0, nil)

	xproto.MapWindow(conn, wid)
	conn.Sync()

	raiseWindow(conn, wid)
	sendAlwaysOnTopMessage(conn, wid, screen.Root)
	conn.Sync()

	log.Infof("Overlay created (window id: %d, depth: %d)", wid, depth)
	return &Overlay{
		conn: conn, win: wid, cmap: cmap, gc: gc,
		screen: screen, width: sW, height: sH,
	}, nil
}

func (o *Overlay) Render(img *image.NRGBA) error {
	bgra := imgToBGRA(img)
	w := int(o.width)
	h := int(o.height)

	maxBytes := 256 * 1024
	rowBytes := w * 4
	stripH := maxBytes / rowBytes
	if stripH < 1 {
		stripH = 1
	}
	if stripH > h {
		stripH = h
	}

	for y := 0; y < h; y += stripH {
		end := y + stripH
		if end > h {
			end = h
		}
		stripRows := end - y
		offset := y * rowBytes
		strip := bgra[offset : offset+stripRows*rowBytes]

		cookie := xproto.PutImageChecked(o.conn, xproto.ImageFormatZPixmap,
			xproto.Drawable(o.win), o.gc,
			uint16(w), uint16(stripRows), 0, int16(y), 0, 32, strip)
		if err := cookie.Check(); err != nil {
			return fmt.Errorf("PutImage strip y=%d failed: %w", y, err)
		}
	}
	o.conn.Sync()
	return nil
}

func (o *Overlay) Width() uint16            { return o.width }
func (o *Overlay) Height() uint16           { return o.height }
func (o *Overlay) Conn() *xgb.Conn          { return o.conn }
func (o *Overlay) Window() xproto.Window    { return o.win }

func (o *Overlay) SetOpacity(opacity float64) {
	atom := internAtom(o.conn, "_NET_WM_WINDOW_OPACITY")
	if atom == 0 {
		return
	}
	val := uint32(float64(0xFFFFFFFF) * opacity)
	data := make([]byte, 4)
	data[0] = byte(val)
	data[1] = byte(val >> 8)
	data[2] = byte(val >> 16)
	data[3] = byte(val >> 24)
	xproto.ChangeProperty(o.conn, xproto.PropModeReplace, o.win,
		atom, xproto.AtomCardinal, 32, 1, data)
}

func (o *Overlay) Hide() {
	xproto.UnmapWindow(o.conn, o.win)
	o.conn.Sync()
}

func (o *Overlay) Close() {
	xproto.DestroyWindow(o.conn, o.win)
	o.conn.Close()
	log.Debug("Overlay closed")
}

func findARGBVisual(conn *xgb.Conn, screen xproto.ScreenInfo) (xproto.Visualid, byte) {
	for _, depth := range screen.AllowedDepths {
		if depth.Depth == 32 {
			for _, vis := range depth.Visuals {
				return vis.VisualId, depth.Depth
			}
		}
	}
	return 0, 0
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	cookie := xproto.InternAtom(conn, false, uint16(len(name)), name)
	reply, err := cookie.Reply()
	if err != nil {
		log.Warnf("Failed to intern atom %s: %v", name, err)
		return 0
	}
	return reply.Atom
}

func setAlwaysOnTopProperty(conn *xgb.Conn, wid xproto.Window) {
	netWmState := internAtom(conn, "_NET_WM_STATE")
	above := internAtom(conn, "_NET_WM_STATE_ABOVE")
	skipTaskbar := internAtom(conn, "_NET_WM_STATE_SKIP_TASKBAR")
	skipPager := internAtom(conn, "_NET_WM_STATE_SKIP_PAGER")

	data := make([]byte, 12)
	putAtom32(data, 0, above)
	putAtom32(data, 4, skipTaskbar)
	putAtom32(data, 8, skipPager)
	xproto.ChangeProperty(conn, xproto.PropModeReplace, wid,
		netWmState, xproto.AtomAtom, 32, 3, data)
}

func setWindowType(conn *xgb.Conn, wid xproto.Window) {
	netWmWindowType := internAtom(conn, "_NET_WM_WINDOW_TYPE")
	notification := internAtom(conn, "_NET_WM_WINDOW_TYPE_NOTIFICATION")
	data := make([]byte, 4)
	putAtom32(data, 0, notification)
	xproto.ChangeProperty(conn, xproto.PropModeReplace, wid,
		netWmWindowType, xproto.AtomAtom, 32, 1, data)
}

func setSticky(conn *xgb.Conn, wid xproto.Window) {
	netWmDesktop := internAtom(conn, "_NET_WM_DESKTOP")
	data := make([]byte, 4)
	data[0] = 0xFF
	data[1] = 0xFF
	data[2] = 0xFF
	data[3] = 0xFF
	xproto.ChangeProperty(conn, xproto.PropModeReplace, wid,
		netWmDesktop, xproto.AtomCardinal, 32, 1, data)
}

func sendAlwaysOnTopMessage(conn *xgb.Conn, wid xproto.Window, root xproto.Window) {
	netWmState := internAtom(conn, "_NET_WM_STATE")
	above := internAtom(conn, "_NET_WM_STATE_ABOVE")

	msg := xproto.ClientMessageEvent{
		Window: wid,
		Type:   netWmState,
		Format: 32,
		Data: xproto.ClientMessageDataUnionData32New([]uint32{
			1, uint32(above), 0, 1, 0,
		}),
	}
	xproto.SendEvent(conn, false, root,
		xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect,
		string(msg.Bytes()))
}

func makeClickThrough(conn *xgb.Conn, wid xproto.Window) {
	region, _ := xfixes.NewRegionId(conn)
	xfixes.CreateRegion(conn, region, []xproto.Rectangle{})
	xfixes.SetWindowShapeRegion(conn, wid, shape.SkInput, 0, 0, region)
	xfixes.DestroyRegion(conn, region)
}

func raiseWindow(conn *xgb.Conn, wid xproto.Window) {
	xproto.ConfigureWindow(conn, wid,
		xproto.ConfigWindowStackMode,
		[]uint32{uint32(xproto.StackModeAbove)})
}

func imgToBGRA(img *image.NRGBA) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	bgra := make([]byte, 4*w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.NRGBAAt(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			i := (y*w + x) * 4
			bgra[i+0] = byte(b >> 8)
			bgra[i+1] = byte(g >> 8)
			bgra[i+2] = byte(r >> 8)
			bgra[i+3] = byte(a >> 8)
		}
	}
	return bgra
}

func putAtom32(data []byte, offset int, atom xproto.Atom) {
	data[offset+0] = byte(atom)
	data[offset+1] = byte(atom >> 8)
	data[offset+2] = byte(atom >> 16)
	data[offset+3] = byte(atom >> 24)
}
