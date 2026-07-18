package wlr

import (
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
)

type BaseProxy = wl.BaseProxy
type Context = wl.Context
type Event = wl.Event
type Surface = wl.Surface
type Output = wl.Output
type Seat = wl.Seat
type XdgPopup = xdg.Popup

func RegistryBindLayerShellInterface(r *wl.Registry, name uint32, version uint32) *ZwlrLayerShellV1 {
	ctx, _ := wl.GetUserData[wl.Context](r)
	l := NewZwlrLayerShellV1(ctx)
	_ = r.Bind(name, "zwlr_layer_shell_v1", version, l)
	return l
}

func RegistryBindVirtualPointerManagerInterface(r *wl.Registry, name uint32, version uint32) *ZwlrVirtualPointerManagerV1 {
	ctx, _ := wl.GetUserData[wl.Context](r)
	v := NewZwlrVirtualPointerManagerV1(ctx)
	_ = r.Bind(name, "zwlr_virtual_pointer_manager_v1", version, v)
	return v
}
