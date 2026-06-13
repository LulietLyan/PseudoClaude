package llm

import "context"

func sendStreamEvent(ctx context.Context, ch chan<- StreamEvent, event StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}
