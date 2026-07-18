package grid

import (
	"image"
	"image/color"
	"testing"

	"mouseless-grid/config"
	"mouseless-grid/keyboard"
)

func cfg(minCellPx int) *config.Config {
	return &config.Config{MinCellPx: minCellPx}
}

func press(code uint16) keyboard.Event {
	return keyboard.Event{Code: code, IsPress: true}
}

func release(code uint16) keyboard.Event {
	return keyboard.Event{Code: code, IsPress: false}
}

func TestNewDefault(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	if g.ScreenW() != 1920 || g.ScreenH() != 1080 {
		t.Fatalf("bad screen size: %dx%d", g.ScreenW(), g.ScreenH())
	}
	if g.State() != StateIdle {
		t.Fatalf("expected Idle, got %d", g.State())
	}
	if g.RegionW() != 1920 || g.RegionH() != 1080 {
		t.Fatalf("bad region: %dx%d", g.RegionW(), g.RegionH())
	}
}

func TestHandleKeyReleaseIgnored(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	if g.HandleKey(release(2)) {
		t.Fatal("release should not be handled")
	}
	if g.State() != StateIdle {
		t.Fatal("state should remain Idle")
	}
}

func TestHandleKeyUnknownIgnored(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	if g.HandleKey(press(999)) {
		t.Fatal("unknown key should not be handled")
	}
}

func TestZoomIdleToZoomed(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	if !g.HandleKey(press(2)) { // key "1" = code 2
		t.Fatal("digit key should be handled")
	}
	if g.State() != StateZoomed {
		t.Fatal("expected Zoomed state")
	}
	// cell 1 → col=0,row=0 → region shrinks to first 1/3
	if g.RegionW() != 640 || g.RegionH() != 360 {
		t.Fatalf("expected 640x360, got %dx%d", g.RegionW(), g.RegionH())
	}
}

func TestZoomIdleToReadyPrecision(t *testing.T) {
	// 100px screen with minCellPx=33 → one zoom into cell 1 makes cellW=33 ≤ 33 → Ready
	// region after zoom = (0,0 33x33), click at center = (16,16)
	g := New(cfg(33), 100, 100)
	g.HandleKey(press(2))
	if g.State() != StateReady {
		t.Fatal("expected Ready when cell width ≤ minCellPx")
	}
	if g.ClickX() != 16 || g.ClickY() != 16 {
		t.Fatalf("click should be center of zoomed region, got (%d,%d)", g.ClickX(), g.ClickY())
	}
}

func TestZoomMultipleLevels(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	// cell 5 (center) = code 6
	g.HandleKey(press(6))
	// cell 5 (center) again
	g.HandleKey(press(6))
	// cell 5 again
	g.HandleKey(press(6))
	if g.State() != StateZoomed {
		t.Fatal("should still be zooming")
	}
	// Each zoom into center: region stays centered, shrinks by 1/3 each time
	// After 3 zooms: 1920/27 ≈ 71, 1080/27 = 40
	if g.RegionW() != 1920/27 || g.RegionH() != 1080/27 {
		t.Fatalf("unexpected region after 3 zooms: %dx%d", g.RegionW(), g.RegionH())
	}
}

func TestEnterEarlyConfirm(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	g.HandleKey(press(2)) // zoom once
	if g.State() != StateZoomed {
		t.Fatal("expected Zoomed")
	}
	// Enter = code 28
	g.HandleKey(press(28))
	if g.State() != StateReady {
		t.Fatal("expected Ready after Enter")
	}
}

func TestEnterInIdleIgnored(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	if g.HandleKey(press(28)) { // Enter with no selections
		t.Fatal("Enter should be ignored in Idle")
	}
}

func TestBackspaceUndo(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	g.HandleKey(press(2)) // zoom 1
	g.HandleKey(press(3)) // zoom 2
	if len(g.selections) != 2 {
		t.Fatal("expected 2 selections")
	}
	// Backspace = code 14
	g.HandleKey(press(14))
	if len(g.selections) != 1 {
		t.Fatal("expected 1 selection after undo")
	}
	if g.State() != StateZoomed {
		t.Fatal("still zoomed after partial undo")
	}
}

func TestBackspaceToIdle(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	g.HandleKey(press(2)) // zoom 1
	g.HandleKey(press(14)) // backspace
	if len(g.selections) != 0 {
		t.Fatal("expected 0 selections")
	}
	if g.State() != StateIdle {
		t.Fatal("expected Idle after undoing last selection")
	}
}

func TestBackspaceFromReady(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	g.HandleKey(press(2))  // zoom
	g.HandleKey(press(28)) // Enter → Ready
	if g.State() != StateReady {
		t.Fatal("expected Ready")
	}
	g.HandleKey(press(14)) // Backspace
	if g.State() != StateIdle {
		t.Fatal("expected Idle after undoing from Ready")
	}
	if len(g.selections) != 0 {
		t.Fatal("expected empty selections")
	}
}

func TestHandleEscape(t *testing.T) {
	g := New(cfg(1), 1920, 1080)
	g.HandleKey(press(2))
	g.HandleKey(press(3))
	g.HandleEscape()
	if g.State() != StateIdle {
		t.Fatal("expected Idle after escape")
	}
	if len(g.selections) != 0 {
		t.Fatal("expected empty selections")
	}
	if g.RegionW() != 1920 || g.RegionH() != 1080 {
		t.Fatal("region should reset to full screen")
	}
}

func TestRegionCorners(t *testing.T) {
	tests := []struct {
		name    string
		cells   []int // 1-9
		wantX, wantY, wantW, wantH int
	}{
		{"cell1", []int{1}, 0, 0, 640, 360},
		{"cell3", []int{3}, 1280, 0, 640, 360},
		{"cell7", []int{7}, 0, 720, 640, 360},
		{"cell9", []int{9}, 1280, 720, 640, 360},
		{"cell5", []int{5}, 640, 360, 640, 360},
		{"1then1", []int{1, 1}, 0, 0, 213, 120},
		{"9then9", []int{9, 9}, 1706, 960, 214, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(cfg(1), 1920, 1080)
			for _, cell := range tt.cells {
				// map cell number (1-9) to key code
				code := uint16(cell + 1) // key "1"=code2, "2"=code3, ...
				g.HandleKey(press(code))
			}
			if g.RegionX() != tt.wantX || g.RegionY() != tt.wantY ||
				g.RegionW() != tt.wantW || g.RegionH() != tt.wantH {
				t.Errorf("got (%d,%d %dx%d), want (%d,%d %dx%d)",
					g.RegionX(), g.RegionY(), g.RegionW(), g.RegionH(),
					tt.wantX, tt.wantY, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestRegionLastRowColFillsRemainder(t *testing.T) {
	// 100px → cells are 33, 33, 34; cell 1 (top-left) = col 0, gets cw=33
	g := New(cfg(1), 100, 100)
	g.HandleKey(press(2)) // cell 1 (top-left)
	if g.RegionW() != 33 {
		t.Fatalf("expected width 33 for cell 1 on 100px screen, got %d", g.RegionW())
	}
}

func TestParseDigit(t *testing.T) {
	tests := []struct {
		alias string
		want  int
	}{
		{"k1", 1}, {"k9", 9}, {"k0", 0},
		{"kp1", 1}, {"kp9", 9},
		{"a", 0}, {"", 0}, {"k10", 0},
	}
	for _, tt := range tests {
		got := parseDigit(tt.alias)
		if got != tt.want {
			t.Errorf("parseDigit(%q) = %d, want %d", tt.alias, got, tt.want)
		}
	}
}

func TestGetAlias(t *testing.T) {
	a, ok := getAlias(2)
	if !ok || a != "k1" {
		t.Fatalf("getAlias(2) = (%q, %v), want (\"k1\", true)", a, ok)
	}
	_, ok = getAlias(999)
	if ok {
		t.Fatal("getAlias(999) should be false")
	}
}

func TestToNRGBA(t *testing.T) {
	c := color.RGBA{100, 150, 200, 255}
	nc := toNRGBA(c, 0.5)
	nrgba, ok := nc.(color.NRGBA)
	if !ok {
		t.Fatal("expected NRGBA")
	}
	if nrgba.R != 100 || nrgba.G != 150 || nrgba.B != 200 {
		t.Fatalf("bad RGB: %d,%d,%d", nrgba.R, nrgba.G, nrgba.B)
	}
	if nrgba.A != 127 { // 255 * 0.5
		t.Fatalf("expected alpha 127, got %d", nrgba.A)
	}
}

func TestRender(t *testing.T) {
	g := New(cfg(10), 100, 100)
	img := g.Render(color.Black, nil, color.White, 1.0)
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Fatalf("render size: %dx%d", bounds.Dx(), bounds.Dy())
	}
	// should not be all one color (grid lines + labels)
	var unique bool
	first := img.NRGBAAt(0, 0)
outer:
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if img.NRGBAAt(x, y) != first {
				unique = true
				break outer
			}
		}
	}
	if !unique {
		t.Fatal("rendered image is all one color, expected grid content")
	}
}

func TestFillRect(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	red := color.NRGBA{255, 0, 0, 255}
	FillRect(img, 2, 2, 4, 4, red)

	// inside rect should be red
	if img.NRGBAAt(3, 3) != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatal("expected red inside filled area")
	}
	// outside should be black (zero value)
	if img.NRGBAAt(1, 1) != (color.NRGBA{0, 0, 0, 0}) {
		t.Fatal("expected transparent outside filled area")
	}
}

func TestFillRectClamp(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	// rect extends beyond image bounds — should not panic
	FillRect(img, -5, -5, 20, 20, color.NRGBA{255, 0, 0, 255})
	if img.NRGBAAt(5, 5).R != 255 {
		t.Fatal("expected red even with out-of-bounds FillRect")
	}
}

func TestDrawHLineVLine(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	white := color.NRGBA{255, 255, 255, 255}
	drawHLine(img, 2, 5, 6, white)
	drawVLine(img, 7, 1, 8, white)

	if img.NRGBAAt(4, 5) != white {
		t.Fatal("h-line pixel not set")
	}
	if img.NRGBAAt(7, 4) != white {
		t.Fatal("v-line pixel not set")
	}
	// outside line range should be transparent
	if img.NRGBAAt(1, 5) == white {
		t.Fatal("pixel outside h-line should not be set")
	}
}

func TestGridString(t *testing.T) {
	g := New(cfg(10), 1920, 1080)
	s := g.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
}

func TestRegionNotDivisibleBy3(t *testing.T) {
	// 100px with minCellPx=0 so we can do multiple zooms
	// cells should be 33, 33, 34; cell 1 (col 0) gets cw=33
	g := New(cfg(0), 100, 100)
	g.HandleKey(press(2)) // cell 1 (0,0)
	if g.RegionW() != 33 {
		t.Fatalf("top-left cell on 100px: expected width 33, got %d", g.RegionW())
	}
	// now region is 33×33; cell 1 again → 33/3=11, last cell 33-22=11
	g.HandleKey(press(9)) // cell 9 (col=2, row=2)
	if g.RegionW() != 11 {
		t.Fatalf("second zoom into cell 9: expected width 11, got %d", g.RegionW())
	}
}
