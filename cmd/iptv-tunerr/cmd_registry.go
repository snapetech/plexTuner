package main

import (
	"flag"

	"github.com/snapetech/iptvtunerr/internal/config"
)

type commandSpec struct {
	Name    string
	Section string
	Summary string
	FlagSet *flag.FlagSet
	Run     func(cfg *config.Config, args []string)
}

var defaultCommandSections = []string{"Core", "Guide/EPG", "VOD"}
var allCommandSections = []string{"Core", "Guide/EPG", "VOD", "Lab/ops"}

func allCommands() []commandSpec {
	commands := append(coreCommands(), setupDoctorCommands()...)
	commands = append(commands, reportCommands()...)
	commands = append(commands, guideReportCommands()...)
	commands = append(commands, vodCommands()...)
	commands = append(commands, opsCommands()...)
	commands = append(commands, catchupOpsCommands()...)
	commands = append(commands, lineupHarvestCommands()...)
	commands = append(commands, oracleOpsCommands()...)
	commands = append(commands, cookieImportCommands()...)
	commands = append(commands, cfStatusCommands()...)
	commands = append(commands, debugBundleCommands()...)
	commands = append(commands, freeSourcesCommands()...)
	commands = append(commands, hdhrScanCommands()...)
	commands = append(commands, liveTVBundleCommands()...)
	commands = append(commands, identityMigrationCommands()...)
	commands = append(commands, plexOpsCommands()...)
	commands = append(commands, plexLabelProxyCommands()...)
	return commands
}

func commandIndex(commands []commandSpec) map[string]commandSpec {
	byName := make(map[string]commandSpec, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}
	return byName
}
