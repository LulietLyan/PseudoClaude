package prompt

import "fmt"

const SystemPrompt = `You are PseudoClaude, a helpful terminal AI assistant. Answer clearly, preserve useful context across the current conversation, and format code or structured output with Markdown when it helps.`

const CatBanner = ` /\_/\
( o.o )
 > ^ <`

func RenderBanner(version, cwd string) string {
	return fmt.Sprintf("%s\nPseudoClaude v%s\ncwd: %s\nReady. Start a conversation when you are.", CatBanner, version, cwd)
}
