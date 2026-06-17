package prompt

import "fmt"

const LogoBanner = `  ██████╗██╗      █████╗ ██╗   ██╗██████╗ ███████╗
 ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██╔════╝
██║     ██║     ███████║██║   ██║██║  ██║█████╗
██║     ██║     ██╔══██║██║   ██║██║  ██║██╔══╝
 ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝███████╗
  ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝

  ██████╗ ██████╗ ██████╗ ███████╗
 ██╔════╝██╔═══██╗██╔══██╗██╔════╝
██║     ██║   ██║██║  ██║█████╗
██║     ██║   ██║██║  ██║██╔══╝
 ╚██████╗╚██████╔╝██████╔╝███████╗
  ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝
                                                 `

const CatBanner = LogoBanner

func RenderBanner(version, cwd string) string {
	return RenderResponsiveBanner(version, cwd, 120, 24)
}

func RenderResponsiveBanner(version, cwd string, width, height int) string {
	logo := SelectLogo(width, height)
	return fmt.Sprintf("%s\nPseudoClaude v%s\ncwd: %s\nReady. Start a conversation when you are.", logo, version, cwd)
}

func SelectLogo(width, height int) string {
	switch {
	case width >= 58 && height >= 12:
		return LogoBanner
	case width >= 34 && height >= 10:
		return MiniLogo
	default:
		return TinyLogo
	}
}

const MiniLogo = `╭─ PseudoClaude ─╮
│ terminal agent │
╰────────────────╯`

const TinyLogo = `PseudoClaude`
