package grid

import (
	"fmt"
	"image"
	"image/color"

	"mouseless-grid/config"
	"mouseless-grid/keyboard"

	log "github.com/sirupsen/logrus"
)

type State int

const (
	StateIdle   State = iota // no selection yet
	StateZoomed              // 1+ selections, still zooming
	StateReady               // precision reached or Enter pressed, waiting for click choice
)

type gridCell struct{ col, row int }

type Grid struct {
	screenW, screenH int
	minCellPx        int
	homeRow          bool
	selections       []gridCell
	regionX, regionY int
	regionW, regionH int
	state            State
	clickX, clickY   int
}

func New(cfg *config.Config, screenW, screenH int) *Grid {
	g := &Grid{
		screenW:  screenW,
		screenH:  screenH,
		minCellPx: cfg.MinCellPx,
		homeRow:   cfg.GridLayout == "home",
		regionW:   screenW,
		regionH:   screenH,
	}
	log.Infof("Grid: %dx%d minCellPx=%d layout=%s", screenW, screenH, g.minCellPx, cfg.GridLayout)
	return g
}

func (g *Grid) HandleKey(ev keyboard.Event) bool {
	if !ev.IsPress {
		return false
	}

	alias, exists := getAlias(ev.Code)
	if !exists {
		return false
	}

	switch g.state {
	case StateIdle, StateZoomed:
		return g.handleZoomKey(alias)
	case StateReady:
		return g.handleReadyKey(alias)
	}
	return false
}

func (g *Grid) handleZoomKey(alias string) bool {
	n := parseDigit(alias)
	if n == 0 && g.homeRow {
		n = parseHomeDigit(alias)
	}
	if n >= 1 && n <= 9 {
		col := (n - 1) % 3
		row := (n - 1) / 3
		g.selections = append(g.selections, gridCell{col, row})
		g.recalculateRegion()
		g.clickX = g.regionX + g.regionW/2
		g.clickY = g.regionY + g.regionH/2
		log.Debugf("Zoom to cell %d (%d,%d) region=(%d,%d %dx%d) click=(%d,%d)",
			n, col, row, g.regionX, g.regionY, g.regionW, g.regionH, g.clickX, g.clickY)

		cellW := g.regionW / 3
		if cellW <= g.minCellPx {
			g.state = StateReady
			log.Debugf("Precision reached: cellW=%d", cellW)
		} else {
			g.state = StateZoomed
		}
		return true
	}

	// enter = confirm early
	if alias == "enter" && len(g.selections) > 0 {
		g.state = StateReady
		g.clickX = g.regionX + g.regionW/2
		g.clickY = g.regionY + g.regionH/2
		log.Debugf("Early confirm, click=(%d,%d)", g.clickX, g.clickY)
		return true
	}

	// backspace = undo last selection
	if alias == "backspace" && len(g.selections) > 0 {
		g.selections = g.selections[:len(g.selections)-1]
		g.recalculateRegion()
		if len(g.selections) == 0 {
			g.state = StateIdle
		}
		log.Debugf("Undo, %d selections left", len(g.selections))
		return true
	}

	return false
}

func (g *Grid) handleReadyKey(alias string) bool {
	if alias == "backspace" && len(g.selections) > 0 {
		g.selections = g.selections[:len(g.selections)-1]
		g.recalculateRegion()
		if len(g.selections) == 0 {
			g.state = StateIdle
		} else {
			g.state = StateZoomed
		}
		log.Debugf("Undo from ready, %d selections left", len(g.selections))
		return true
	}
	return false
}

func (g *Grid) recalculateRegion() {
	g.regionX, g.regionY = 0, 0
	g.regionW, g.regionH = g.screenW, g.screenH
	for _, s := range g.selections {
		cw := g.regionW / 3
		ch := g.regionH / 3
		g.regionX += s.col * cw
		g.regionY += s.row * ch
		if s.col == 2 {
			g.regionW -= 2 * cw
		} else {
			g.regionW = cw
		}
		if s.row == 2 {
			g.regionH -= 2 * ch
		} else {
			g.regionH = ch
		}
	}
}

func (g *Grid) HandleEscape() {
	g.selections = nil
	g.regionX, g.regionY = 0, 0
	g.regionW, g.regionH = g.screenW, g.screenH
	g.state = StateIdle
	log.Debug("Selection cancelled")
}

// --- state queries ---

func (g *Grid) State() State     { return g.state }
func (g *Grid) ClickX() int      { return g.clickX }
func (g *Grid) ClickY() int      { return g.clickY }
func (g *Grid) ScreenW() int     { return g.screenW }
func (g *Grid) ScreenH() int     { return g.screenH }
func (g *Grid) RegionX() int     { return g.regionX }
func (g *Grid) RegionY() int     { return g.regionY }
func (g *Grid) RegionW() int     { return g.regionW }
func (g *Grid) RegionH() int     { return g.regionH }

// --- rendering ---

func (g *Grid) Render(bgColor, textColor, highlightColor color.Color, opacity float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, g.screenW, g.screenH))

	bgRGBA := toNRGBA(bgColor, opacity)
	lineColor := toNRGBA(color.RGBA{255, 255, 255, 255}, 0.3)
	labelColor := toNRGBA(color.RGBA{255, 255, 255, 255}, 1.0)
	highlightBg := toNRGBA(highlightColor, opacity)

	// fill entire screen with bg
	FillRect(img, 0, 0, g.screenW, g.screenH, bgRGBA)

	cellW := g.regionW / 3
	cellH := g.regionH / 3
	homeLabels := []string{"A", "S", "D", "F", "G", "H", "J", "K", "L"}

	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			x := g.regionX + col*cellW
			y := g.regionY + row*cellH
			// extend last row/col to fill any remainder pixels
			w := cellW
			if col == 2 {
				w = g.regionX + g.regionW - x
			}
			h := cellH
			if row == 2 {
				h = g.regionY + g.regionH - y
			}
			n := row*3 + col + 1

			// highlight selected cell (last selection)
			if g.state != StateIdle && len(g.selections) > 0 {
				last := g.selections[len(g.selections)-1]
				if col == last.col && row == last.row {
					FillRect(img, x, y, w, h, highlightBg)
				} else {
					FillRect(img, x, y, w, h, bgRGBA)
				}
			} else {
				FillRect(img, x, y, w, h, bgRGBA)
			}

			// grid lines
			drawHLine(img, x, y, w, lineColor)
			drawVLine(img, x, y, h, lineColor)

			// label
			label := fmt.Sprintf("%d", n)
			if g.homeRow {
				label = homeLabels[n-1]
			}
			scale := 2
			if cellW > 80 {
				scale = 4
			} else if cellW > 40 {
				scale = 3
			}
			DrawString(img, label, x+cellW/2, y+cellH/2, scale, labelColor)
		}
	}

	return img
}

// --- helpers ---

func parseHomeDigit(alias string) int {
	switch alias {
	case "a":
		return 1
	case "s":
		return 2
	case "d":
		return 3
	case "f":
		return 4
	case "g":
		return 5
	case "h":
		return 6
	case "j":
		return 7
	case "k":
		return 8
	case "l":
		return 9
	}
	return 0
}

func parseDigit(alias string) int {
	if len(alias) == 2 && alias[0] == 'k' && alias[1] >= '1' && alias[1] <= '9' {
		return int(alias[1] - '0')
	}
	if len(alias) == 3 && alias[0] == 'k' && alias[1] == 'p' && alias[2] >= '1' && alias[2] <= '9' {
		return int(alias[2] - '0')
	}
	return 0
}

func toNRGBA(c color.Color, opacity float64) color.Color {
	r, g, b, a := c.RGBA()
	return color.NRGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(float64(a>>8) * opacity),
	}
}

func FillRect(img *image.NRGBA, x, y, w, h int, c color.Color) {
	for py := y; py < y+h && py < img.Bounds().Dy(); py++ {
		for px := x; px < x+w && px < img.Bounds().Dx(); px++ {
			img.Set(px, py, c)
		}
	}
}

func drawHLine(img *image.NRGBA, x, y, w int, c color.Color) {
	for px := x; px < x+w && px < img.Bounds().Dx(); px++ {
		if y >= 0 && y < img.Bounds().Dy() {
			img.Set(px, y, c)
		}
	}
}

func drawVLine(img *image.NRGBA, x, y, h int, c color.Color) {
	for py := y; py < y+h && py < img.Bounds().Dy(); py++ {
		if x >= 0 && x < img.Bounds().Dx() {
			img.Set(x, py, c)
		}
	}
}

func getAlias(code uint16) (string, bool) {
	var keyAliases = map[uint16]string{
		1: "esc", 2: "k1", 3: "k2", 4: "k3", 5: "k4", 6: "k5",
		7: "k6", 8: "k7", 9: "k8", 10: "k9", 11: "k0",
		12: "minus", 13: "equal", 14: "backspace", 15: "tab",
		16: "q", 17: "w", 18: "e", 19: "r", 20: "t",
		21: "y", 22: "u", 23: "i", 24: "o", 25: "p",
		26: "leftbrace", 27: "rightbrace", 28: "enter", 29: "leftctrl",
		30: "a", 31: "s", 32: "d", 33: "f", 34: "g",
		35: "h", 36: "j", 37: "k", 38: "l",
		39: "semicolon", 40: "apostrophe", 41: "grave",
		42: "leftshift", 43: "backslash",
		44: "z", 45: "x", 46: "c", 47: "v", 48: "b",
		49: "n", 50: "m", 51: "comma", 52: "dot", 53: "slash",
		54: "rightshift", 55: "kpasterisk", 56: "leftalt", 57: "space",
		58: "capslock",
		59: "f1", 60: "f2", 61: "f3", 62: "f4", 63: "f5", 64: "f6",
		65: "f7", 66: "f8", 67: "f9", 68: "f10",
		69: "numlock", 70: "scrolllock",
		71: "kp7", 72: "kp8", 73: "kp9", 74: "kpminus",
		75: "kp4", 76: "kp5", 77: "kp6", 78: "kpplus",
		79: "kp1", 80: "kp2", 81: "kp3", 82: "kp0", 83: "kpdot",
		87: "f11", 88: "f12",
		96: "kpenter", 97: "rightctrl", 98: "kpslash", 100: "rightalt",
		102: "home", 103: "up", 104: "pageup",
		105: "left", 106: "right", 107: "end", 108: "down", 109: "pagedown",
		110: "insert", 111: "delete",
		113: "mute", 114: "volumedown", 115: "volumeup",
		125: "leftmeta", 126: "rightmeta",
	}
	alias, ok := keyAliases[code]
	return alias, ok
}

func (g *Grid) String() string {
	return fmt.Sprintf("Grid: %dx%d selections=%d state=%d region=(%d,%d %dx%d)",
		g.screenW, g.screenH, len(g.selections), g.state,
		g.regionX, g.regionY, g.regionW, g.regionH)
}
