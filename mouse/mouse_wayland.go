package mouse

import (
	"mouseless-grid/overlay"
	"mouseless-grid/wlr"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	btnLeft       = 0x110
	btnRight      = 0x111
	btnStatePress = 1
	btnStateRelease = 0
)

type WaylandMouse struct {
	virtPtr *wlr.ZwlrVirtualPointerV1
}

func NewWayland() (*WaylandMouse, error) {
	state := overlay.WlState()
	vp, err := state.VirtPtrMgr().CreateVirtualPointer(state.Seat())
	if err != nil {
		return nil, err
	}

	log.Debug("Created Wayland virtual pointer")
	return &WaylandMouse{virtPtr: vp}, nil
}

func (m *WaylandMouse) MoveTo(x, y int32) error {
	log.Debugf("MoveTo: (%d, %d)", x, y)
	state := overlay.WlState()
	m.virtPtr.MotionAbsolute(uint32(time.Now().UnixMilli()),
		uint32(x), uint32(y), state.ScreenWidth(), state.ScreenHeight())
	m.virtPtr.Frame()
	return nil
}

func (m *WaylandMouse) ClickAt(x, y int32) error {
	m.MoveTo(x, y)
	now := uint32(time.Now().UnixMilli())
	log.Debugf("Left click at (%d,%d)", x, y)
	m.virtPtr.Button(now, btnLeft, btnStatePress)
	m.virtPtr.Frame()
	m.virtPtr.Button(now+1, btnLeft, btnStateRelease)
	m.virtPtr.Frame()
	return nil
}

func (m *WaylandMouse) RightClickAt(x, y int32) error {
	m.MoveTo(x, y)
	now := uint32(time.Now().UnixMilli())
	log.Debugf("Right click at (%d,%d)", x, y)
	m.virtPtr.Button(now, btnRight, btnStatePress)
	m.virtPtr.Frame()
	m.virtPtr.Button(now+1, btnRight, btnStateRelease)
	m.virtPtr.Frame()
	return nil
}

func (m *WaylandMouse) Close() error {
	return nil
}
