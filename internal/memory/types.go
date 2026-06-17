package memory

import (
	"time"

	"PseudoClaude/internal/llm"
)

type Level string
type NoteType string

const (
	LevelProject Level = "project"
	LevelUser    Level = "user"

	TypeUserPreference     NoteType = "user_preference"
	TypeCorrectionFeedback NoteType = "correction_feedback"
	TypeProjectKnowledge   NoteType = "project_knowledge"
	TypeReferenceMaterial  NoteType = "reference_material"

	IndexFileName = "MEMORY.md"
	MaxIndexLines = 200
	MaxIndexBytes = 25 * 1024
)

type Note struct {
	Type    NoteType
	Title   string
	Content string
	Created time.Time
	Updated time.Time
}

type Operation struct {
	Action   string   `json:"action"`
	Level    Level    `json:"level"`
	Type     NoteType `json:"type,omitempty"`
	Title    string   `json:"title,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Slug     string   `json:"slug,omitempty"`
	Filename string   `json:"filename,omitempty"`
	Content  string   `json:"content,omitempty"`
}

type UpdateInput struct {
	Messages []llm.Message
}
