package task

import (
	"fmt"
	"strings"
)

const notificationLimit = 2000

func FormatNotification(snapshot Snapshot) string {
	body := strings.Builder{}
	body.WriteString("<task-notification>\n")
	body.WriteString(fmt.Sprintf("id: %s\n", snapshot.ID))
	if snapshot.Name != "" {
		body.WriteString(fmt.Sprintf("name: %s\n", snapshot.Name))
	}
	if snapshot.Type != "" {
		body.WriteString(fmt.Sprintf("type: %s\n", snapshot.Type))
	}
	body.WriteString(fmt.Sprintf("status: %s\n", snapshot.Status))
	switch snapshot.Status {
	case StatusCompleted, StatusMaxTurns:
		if snapshot.Result != "" {
			body.WriteString("result: " + truncate(snapshot.Result, notificationLimit) + "\n")
		}
	case StatusFailed, StatusCancelled:
		if snapshot.Error != "" {
			body.WriteString("error: " + truncate(snapshot.Error, notificationLimit) + "\n")
		}
	}
	body.WriteString("</task-notification>")
	return body.String()
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
