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
		Name:    "actor",
		Summary: "Declare, edit and remove actors — who can initiate a flow or a process.",
		Sub: []*Command{
			{
				Name:    "add",
				Summary: "Declare an actor.",
				Usage:   "openrecord actor add ID --kind human|external-system|time --description TEXT [--alias WORD]",
				Flags:   describes(actorAddFlags),
				Run:     runActorAdd,
			},
			{
				Name:    "edit",
				Summary: "Replace an actor's kind, description and/or aliases.",
				Usage:   "openrecord actor edit ID [--kind KIND] [--description TEXT] [--alias WORD]",
				Flags:   describes(actorEditFlags),
				Run:     runActorEdit,
			},
			{
				Name:    "remove",
				Summary: "Remove an actor, when nothing references it.",
				Usage:   "openrecord actor remove ID",
				Run:     runActorRemove,
			},
		},
	},
	{
		Name:    "check",
		Summary: "Declare and remove checks — task 09's registry of what verifies a decision or a spec.",
		Sub: []*Command{
			{
				Name:    "add",
				Summary: "Declare a check: a command, the decision that justifies it, and what it enforces.",
				Usage:   "openrecord check add ID --command CMD --justified-by PATH [--enforces PATH-OR-COORDINATE] [--config PATH] [--needs SANDBOX-ID]",
				Flags:   describes(checkAddFlags),
				Run:     runCheckAdd,
			},
			{
				Name:    "remove",
				Summary: "Remove a check.",
				Usage:   "openrecord check remove ID",
				Run:     runCheckRemove,
			},
		},
	},
	{
		Name:    "sandbox",
		Summary: "Declare, edit and remove sandboxes — task 11's registry of how to bring up an environment to observe the system running.",
		Sub: []*Command{
			{
				Name:    "add",
				Summary: "Declare a sandbox: how to bring up an environment, and how to know it is ready, read its output and tear it down.",
				Usage: "openrecord sandbox add ID --description TEXT [--component ID] [--setup CMD] [--start CMD] " +
					"--ready-http URL|--ready-log LINE [--ready-timeout DURATION] [--exercise CMD] [--approximates PATH#ANCHOR] " +
					"[--observes PATH] --logs-source CMD [--logs-format FORMAT] --stop CMD",
				Flags: describes(sandboxFlags),
				Run:   runSandboxAdd,
			},
			{
				Name:    "edit",
				Summary: "Replace any field of a sandbox except its id.",
				Usage:   "openrecord sandbox edit ID [the same flags sandbox add takes]",
				Flags:   describes(sandboxFlags),
				Run:     runSandboxEdit,
			},
			{
				Name:    "remove",
				Summary: "Remove a sandbox, when no check still needs it.",
				Usage:   "openrecord sandbox remove ID",
				Run:     runSandboxRemove,
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
		Summary: "Rank every scoped record against a context with Jev, and store the result for review.",
		Usage:   "openrecord search --for COORDINATE --literal TERM --semantic QUERY --context TEXT [--omit PATH]",
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
		Usage:   "openrecord skills --emit DIR [--dry-run] [--guard]",
		Flags:   describes(skillsFlags),
		Run:     runSkills,
	},
	{
		Name:    "hook",
		Summary: "Claude Code integration: decide a PreToolUse hook's allow/deny.",
		Sub: []*Command{
			{
				Name:    "check",
				Summary: "Read a PreToolUse tool call as JSON on stdin and print Claude Code's decision JSON on stdout.",
				Usage:   "openrecord hook check   (reads the tool call as JSON on stdin; takes no flags or arguments)",
				Run:     runHookCheck,
			},
		},
	},
	{
		Name:    "concerns",
		Summary: "Print the concerns catalogue: what kinds of decision are worth recording.",
		Usage:   "openrecord concerns [--json]",
		Flags:   describes(concernsFlags),
		Run:     runConcerns,
	},
	{
		Name:    "verification",
		Summary: "Print the verification catalogue: how a record's claim would be checked.",
		Usage:   "openrecord verification [KEY]",
		Run:     runVerification,
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
			{
				Name:    "index",
				Summary: "Register and embed this project's collections, incrementally.",
				Usage:   "openrecord qmd index [--rebuild]",
				Flags:   describes(qmdIndexFlags),
				Run:     runQmdIndex,
			},
		},
	},
	{
		Name:    "jev",
		Summary: "Report on the Jev decisions model that ranks search.",
		Sub: []*Command{
			{
				Name:    "status",
				Summary: "Whether the key is present and the endpoint is reachable.",
				Usage:   "openrecord jev status",
				Run:     runJevStatus,
			},
		},
	},
	{
		Name:    "review",
		Summary: "Open, mark and check a stored search's review.",
		Sub: []*Command{
			{
				Name:    "open",
				Summary: "Print a search's record and mark it opened.",
				Usage:   "openrecord review open ID PATH",
				Run:     runReviewOpen,
			},
			{
				Name:    "mark",
				Summary: "Store a verdict for a record that was opened earlier.",
				Usage:   "openrecord review mark ID PATH --verdict governs|contradicts|unrelated",
				Flags:   describes(reviewMarkFlags),
				Run:     runReviewMark,
			},
			{
				Name:    "status",
				Summary: "Report each served record's verdict and whether the search is complete.",
				Usage:   "openrecord review status ID",
				Run:     runReviewStatus,
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
