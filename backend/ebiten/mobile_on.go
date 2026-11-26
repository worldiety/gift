//go:build android || ios

package ebiten

// mobilePlatform reports that Ebitengine runs as a view the operating system
// drives, not as a window this program opens; see [NewGame].
const mobilePlatform = true
