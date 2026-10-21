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
			{
				Name:    "run",
				Summary: "Run a check, or every check whose `enforces` covers a set of files — task 13b, the binary enforcing what impact only reports.",
				Usage:   "openrecord check run [ID] [--files FILE...] [--per-change]",
				Flags:   describes(checkRunFlags),
				Run:     runCheckRun,
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
			{
				Name:    "run",
				Summary: "Run a sandbox's lifecycle, or every sandbox a set of files' records call for — task 13b.",
				Usage:   "openrecord sandbox run [ID] [--files FILE...]",
				Flags:   describes(sandboxRunFlags),
				Run:     runSandboxRun,
			},
		},
	},
	{
		Name:    "compliance",
		Summary: "Report and log task 14's compliance registry — whether a record's code is known to still hold, never the fact of it.",
		Sub: []*Command{
			{
				Name:    "status",
				Summary: "Report every record's compliance state, or only the ones governing a set of files.",
				Usage:   "openrecord compliance status [--files FILE...]",
				Flags:   describes(complianceStatusFlags),
				Run:     runComplianceStatus,
			},
			{
				Name:    "log",
				Summary: "Log a compliance event for one record: implemented, adopted, verified, or blocked.",
				Usage: "openrecord compliance log PATH --event implemented|adopted|verified|blocked " +
					"[--verified-with checks,scenarios,sandbox,audit] [--blocked-by pending:RECORD|contradicts:RECORD[:NOTE]|missing:NOTE]",
				Flags: describes(complianceLogFlags),
				Run:   runComplianceLog,
			},
		},
	},
	{
		Name:    "run",
		Summary: "Task 15's core: a run's own artifacts — scope, plan, tasks and gaps — never records.",
		Sub: []*Command{
			{
				Name:    "start",
				Summary: "Compute a plan over a scope and open a run.",
				Usage:   "openrecord run start --scope PATH...",
				Flags:   describes(runStartFlags),
				Run:     runRunStart,
			},
			{
				Name:    "status",
				Summary: "Report a run's own state, its tasks, and drift since its plan was computed.",
				Usage:   "openrecord run status RUN-ID",
				Run:     runRunStatus,
			},
			{
				Name:    "list",
				Summary: "List every run under .openrecord/runs/.",
				Usage:   "openrecord run list",
				Run:     runRunList,
			},
			{
				Name:    "tasks",
				Summary: "Plan and replan a run's tasks.",
				Sub: []*Command{
					{
						Name:    "set",
						Summary: "Validate and save a run's task list, read as JSON from stdin or --file.",
						Usage:   "openrecord run tasks set RUN-ID [--file PATH]   (reads the task list from stdin otherwise)",
						Flags:   describes(runTasksSetFlags),
						Run:     runRunTasksSet,
					},
				},
			},
			{
				Name:    "task",
				Summary: "Move one task through its own lifecycle.",
				Sub: []*Command{
					{
						Name:    "start",
						Summary: "Move a pending task to in_progress.",
						Usage:   "openrecord run task start RUN-ID TASK-ID",
						Run:     runRunTaskStart,
					},
					{
						Name:    "done",
						Summary: "Run the task's checks against whatever it changed, and mark it done if they all pass.",
						Usage:   "openrecord run task done RUN-ID TASK-ID",
						Run:     runRunTaskDone,
					},
				},
			},
			{
				Name:    "gaps",
				Summary: "List every open gap, across every run.",
				Usage:   "openrecord run gaps",
				Run:     runRunGaps,
			},
			{
				Name:    "gap",
				Summary: "Register and resolve a run's own gaps.",
				Sub: []*Command{
					{
						Name:    "add",
						Summary: "Register a gap: a question only a person can answer.",
						Usage: "openrecord run gap add RUN-ID --id ID --question TEXT --kind missing|pending|contradicts " +
							"[--record PATH] --evidence REF... [--blocks TASK-ID...]",
						Flags: describes(runGapAddFlags),
						Run:   runRunGapAdd,
					},
					{
						Name:    "resolve",
						Summary: "Resolve a gap with an existing, accepted record.",
						Usage:   "openrecord run gap resolve RUN-ID GAP-ID --record PATH",
						Flags:   describes(runGapResolveFlags),
						Run:     runRunGapResolve,
					},
				},
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
		Name:    "impact",
		Summary: "Report a record's full neighbourhood, or — with --files — which decisions and specs govern a set of code files.",
		Usage:   "openrecord impact PATH [--only SECTIONS] | openrecord impact --files FILE... [--only SECTIONS]",
		Flags:   describes(impactFlags),
		Run:     runImpact,
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
