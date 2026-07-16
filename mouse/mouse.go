package mouse

import (
	"fmt"
	"time"

	"github.com/bendahl/uinput"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	log "github.com/sirupsen/logrus"
)

type VirtualMouse struct {
	conn   *xgb.Conn
	screen xproto.ScreenInfo
	device uinput.Mouse
}

func New(conn *xgb.Conn) (*VirtualMouse, error) {
	screen := xproto.Setup(conn).Roots[0]
	device, err := uinput.CreateMouse("/dev/uinput", []byte("mouseless-grid"))
	if err != nil {
		return nil, fmt.Errorf("failed to create uinput mouse: %w", err)
	}
	log.Debug("Created uinput mouse device + X11 WarpPointer")
	return &VirtualMouse{conn: conn, screen: screen, device: device}, nil
}

func (m *VirtualMouse) MoveTo(x, y int32) error {
	log.Debugf("MoveTo: (%d, %d)", x, y)
	xproto.WarpPointer(m.conn, xproto.WindowNone, m.screen.Root,
		0, 0, 0, 0, int16(x), int16(y))
	m.conn.Sync()
	time.Sleep(15 * time.Millisecond)
	return nil
}

func (m *VirtualMouse) ClickAt(x, y int32) error {
	m.MoveTo(x, y)
	return m.device.LeftClick()
}

func (m *VirtualMouse) RightClickAt(x, y int32) error {
	m.MoveTo(x, y)
	return m.device.RightClick()
}

func (m *VirtualMouse) Close() error {
	return m.device.Close()
}
