package hook

import "fmt"

func formatString(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
