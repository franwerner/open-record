package cli

// commands is the whole command surface. Every entry is declared from the
// start, even before it runs, so `openrecord` shows what the tool is rather than
// what has been built so far.
var commands = []*Command{
	{
		Name:    "map",
		Summary: "Navigate the store, one level at a time.",
		Usage:   "openrecord map [--for COORDINATE]",
		Flags:   describes(coordinateFlag),
		Run:     runMap,
	},
	{
		Name:    "component",
		Summary: "Declare, remove and resolve components.",
		Sub: []*Command{
			{
				Name:    "add",
				Summary: "Declare a surface and create its folder.",
				Usage:   "openrecord component add ID --path DIR --title TITLE --description TEXT",
				Flags:   describes(componentAddFlags),
				Run:     runComponentAdd,
			},
			{
				Name:    "remove",
				Summary: "Remove a surface, when nothing references it.",
				Usage:   "openrecord component remove ID",
				Run:     runComponentRemove,
			},
			{
				Name:    "owners",
				Summary: "Report which component owns a repository path.",
				Usage:   "openrecord component owners PATH",
				Run:     runComponentOwners,
			},
		},
	},
	{
		Name:    "level",
		Summary: "Create a concern or subgroup with its index.",
		Sub: []*Command{
			{
				Name:    "add",
				Summary: "Create a level, defaulting its description from the catalogue.",
				Usage:   "openrecord level add COORDINATE [--title TITLE] [--description TEXT]",
				Flags:   describes(levelAddFlags),
				Run:     runLevelAdd,
			},
		},
	},
	{
		Name:    "record",
		Summary: "Write and edit records.",
		Sub: []*Command{
			{
				Name:    "write",
				Summary: "Write a record whole, validating before it lands.",
				Usage:   "openrecord record write PATH --title TITLE --description TEXT --status STATUS --body-file FILE [--components ID]",
				Flags:   describes(recordWriteFlags),
				Run:     runRecordWrite,
			},
			{
				Name:    "edit",
				Summary: "Replace one section, revalidating the whole file.",
				Usage:   "openrecord record edit PATH --section HEADING --body-file FILE",
				Flags:   describes(recordEditFlags),
				Run:     runRecordEdit,
			},
		},
	},
	{
		Name:    "validate",
		Summary: "Check a coordinate is well-formed, recursively.",
		Usage:   "openrecord validate [--for COORDINATE]",
		Flags:   describes(coordinateFlag),
		Run:     runValidate,
	},
	{
		Name:    "grep",
		Summary: "Literal search, scoped to a coordinate.",
		Usage:   "openrecord grep TERM --for COORDINATE",
		Flags:   describes(coordinateFlag),
		Run:     runGrep,
	},
	{
		Name:    "search",
		Summary: "Literal search plus meaning, scoped to a coordinate, merged into one list.",
		Usage:   "openrecord search TERM --for COORDINATE [--omit PATH]",
		Flags:   describes(searchFlags),
		Run:     runSearch,
	},
	{
		Name:    "diagram",
		Summary: "Emit a spec as Mermaid on stdout.",
		Usage:   "openrecord diagram PATH",
		Run:     runDiagram,
	},
	{
		Name:    "skills",
		Summary: "Emit the bundled agent skills into a directory.",
		Usage:   "openrecord skills --emit DIR [--with-qmd] [--dry-run]",
		Flags:   describes(skillsFlags),
		Run:     runSkills,
	},
	{
		Name:    "concerns",
		Summary: "Print the concerns catalogue: what kinds of decision are worth recording.",
		Usage:   "openrecord concerns [--json]",
		Flags:   describes(concernsFlags),
		Run:     runConcerns,
	},
	{
		Name:    "qmd",
		Summary: "Report on semantic search, and install it.",
		Sub: []*Command{
			{
				Name:    "status",
				Summary: "Whether qmd is installed, and the collections this project needs.",
				Usage:   "openrecord qmd status",
				Run:     runQmdStatus,
			},
			{
				Name:    "install",
				Summary: "Install qmd, so records can be found by meaning.",
				Usage:   "openrecord qmd install [--force]",
				Flags:   describes(qmdInstallFlags),
				Run:     runQmdInstall,
			},
		},
	},
	{
		Name:    "version",
		Summary: "Print the build and the store format it understands.",
		Usage:   "openrecord version",
		Run:     runVersion,
	},
}
