package command

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		empty   bool
		slash   bool
		token   string
		args    string
		trimmed string
	}{
		{name: "empty", input: " \t\n ", empty: true},
		{name: "plain", input: " hello ", trimmed: "hello"},
		{name: "upper", input: "/HELP", slash: true, token: "/help", trimmed: "/HELP"},
		{name: "args", input: "/help   arg value  ", slash: true, token: "/help", args: "arg value", trimmed: "/help   arg value"},
		{name: "slash only", input: "/", slash: true, token: "/", trimmed: "/"},
		{name: "tab", input: "/status\tfull", slash: true, token: "/status", args: "full", trimmed: "/status\tfull"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Empty != tt.empty || got.IsSlash != tt.slash || got.Token != tt.token || got.Args != tt.args || got.Input != tt.trimmed {
				t.Fatalf("Parse() = %+v", got)
			}
		})
	}
}
