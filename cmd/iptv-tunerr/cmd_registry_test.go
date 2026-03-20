package main

import "testing"

func TestAllCommandsUniqueAndSectioned(t *testing.T) {
	commands := allCommands()
	if len(commands) == 0 {
		t.Fatal("expected commands")
	}
	allowedSections := map[string]bool{}
	for _, section := range defaultCommandSections {
		allowedSections[section] = true
	}
	seen := map[string]bool{}
	for _, cmd := range commands {
		if cmd.Name == "" {
			t.Fatalf("empty command name: %+v", cmd)
		}
		if seen[cmd.Name] {
			t.Fatalf("duplicate command %q", cmd.Name)
		}
		seen[cmd.Name] = true
		if !allowedSections[cmd.Section] {
			t.Fatalf("command %q has unknown section %q", cmd.Name, cmd.Section)
		}
		if cmd.Summary == "" {
			t.Fatalf("command %q missing summary", cmd.Name)
		}
		if cmd.Run == nil {
			t.Fatalf("command %q missing Run", cmd.Name)
		}
		if cmd.FlagSet != nil && cmd.FlagSet.Name() == "" {
			t.Fatalf("command %q has unnamed FlagSet", cmd.Name)
		}
	}
}
