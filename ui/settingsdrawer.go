package ui

import "github.com/hajimehoshi/ebiten/v2"

// SettingsDrawer slide out modal menu
type SettingsDrawer struct {
	DrawerBase
}

// NewSettingsDrawer creates the SettingsDrawer object; it starts life off screen to the left
func NewSettingsDrawer() *SettingsDrawer {
	// according to https://material.io/components/navigation-drawer#specs, always 256 wide
	d := &SettingsDrawer{DrawerBase: DrawerBase{WindowBase: WindowBase{width: 360, height: 0, x: -400, y: ToolbarHeight}}}
	return d
}

// ShowSettingsDrawer makes the card back picker visible
func (u *UI) ShowSettingsDrawer(booleanSettings map[string]bool) {
	con := u.VisibleDrawer()
	if con == u.settingsDrawer {
		return
	}
	if con != nil {
		con.Hide()
	}
	u.settingsDrawer.widgets = u.settingsDrawer.widgets[:0]
	u.settingsDrawer.widgets = []Widgety{
		// widget x, y will be set by LayoutWidgets()
		NewNavItem(u.settingsDrawer, "", "speed", "Card speed...", ebiten.KeyA),
		// NewCheckbox(u.settingsDrawer, "", "Fixed cards", booleanSettings["FixedCards"]),
		NewCheckbox(u.settingsDrawer, "", "Power moves", booleanSettings["PowerMoves"]),
		NewCheckbox(u.settingsDrawer, "", "Colorful cards", booleanSettings["ColorfulCards"]),
		NewCheckbox(u.settingsDrawer, "", "Show movable cards", booleanSettings["ShowMovableCards"]),
		NewCheckbox(u.settingsDrawer, "", "Mirror baize", booleanSettings["MirrorBaize"]),
		NewCheckbox(u.settingsDrawer, "", "Mute sounds", booleanSettings["Mute"]),
		NewCheckbox(u.settingsDrawer, "", "Safe collect", booleanSettings["SafeCollect"]),
	}
	u.settingsDrawer.LayoutWidgets()
	u.settingsDrawer.Show()
}

func (u *UI) ShowAniSpeedDrawer(aniSpeed float64) {
	con := u.VisibleDrawer()
	if con == u.aniSpeedDrawer {
		return
	}
	if con != nil {
		con.Hide()
	}
	u.aniSpeedDrawer.widgets = u.settingsDrawer.widgets[:0]
	u.aniSpeedDrawer.widgets = []Widgety{
		NewText(u.aniSpeedDrawer, "aniTitle", "Card Animation Speed"),
		NewRadioButton(u.aniSpeedDrawer, "aniFast", "Fast", aniSpeed < 0.6),
		NewRadioButton(u.aniSpeedDrawer, "aniNormal", "Normal", aniSpeed == 0.6),
		NewRadioButton(u.aniSpeedDrawer, "aniSlow", "Slow", aniSpeed > 0.6),
	}
	u.aniSpeedDrawer.LayoutWidgets()
	u.aniSpeedDrawer.Show()
}
