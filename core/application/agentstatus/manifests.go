package agentstatus

// 組み込みマニフェスト。herdr の distribution/agent-detection/{claude,codex}.toml
// (claude 2026.09.04.1 / codex 2026.09.05.1)を移植したもの。ルール ID・優先度・
// 領域は原典と同じにしてあるので、原典の更新を追従しやすい。

var claudeManifest = Manifest{
	Tool: "claude",
	Rules: []Rule{
		{
			ID: "osc_title_working", State: StateWorking, Priority: 1100, Region: "osc_title",
			VisibleWorking: true,
			// Braille は 2.1.227 以前、半円は 2.1.228 以降のスピナー。
			Gate: Gate{Regex: []string{`^[\x{2800}-\x{28FF}\x{25D0}-\x{25D3}] `}},
		},
		{
			ID: "live_turn_working", State: StateWorking, Priority: 970, Region: "bottom_non_empty_lines(12)",
			VisibleWorking: true,
			Gate: Gate{Any: []Gate{
				{LineRegex: []string{`^\s*[⏸⏵].*esc to interrupt(?:\s|·|$)`}},
				{LineRegex: []string{`^\s*[\x{002A}\x{00B7}\x{2722}\x{2736}\x{273B}\x{273D}]\s+\S.*…(?:\s+\(\d+[smh](?:\s|·)|\s*$)`}},
			}},
		},
		{
			ID: "background_agents_working", State: StateWorking, Priority: 965, Region: "last_non_empty_above_prompt_box",
			VisibleWorking: true,
			Gate:           Gate{LineRegex: []string{`^\s*[\x{002A}\x{00B7}\x{2722}\x{2736}\x{273B}\x{273D}]\s+Waiting for [1-9]\d* background agents? to finish\s*$`}},
		},
		{
			ID: "background_mcp_task_working", State: StateWorking, Priority: 965, Region: "bottom_non_empty_lines(12)",
			VisibleWorking: true,
			// claude は活動サマリを 0 桁目に描画し、折り返し行はインデントされる。
			Gate: Gate{
				Regex: []string{`(?m)^[\x{002A}\x{00B7}\x{2722}\x{2736}\x{273B}\x{273D}][ \t]+\S[^\n]*?(?:\n[ \t]+[^\n]*?){0,3}·(?:[ \t]+|\n[ \t]*)[1-9]\d*(?:[ \t]+|\n[ \t]*)MCP(?:[ \t]+|\n[ \t]*)tasks?(?:[ \t]+|\n[ \t]*)still(?:[ \t]+|\n[ \t]*)running[ \t]*$`},
				Not: []Gate{
					{Contains: []string{"do you want to proceed?"}},
					{Contains: []string{"esc to cancel"}},
					{Contains: []string{"waiting for permission"}},
					{Contains: []string{"do you want to allow this connection?"}},
					{Contains: []string{"tab to amend"}},
					{Contains: []string{"ctrl+e to explain"}},
				},
			},
		},
		{
			ID: "btw_overlay_working", State: StateWorking, Priority: 975, Region: "bottom_non_empty_lines(5)",
			VisibleWorking: true,
			Gate:           Gate{LineRegex: []string{`^\s*/btw(?:\s|$)`, `(?i)esc to close\s*$`}},
		},
		{
			ID: "transcript_viewer", State: StateUnknown, Priority: 1000, Region: "bottom_non_empty_lines(3)",
			SkipStateUpdate: true,
			Gate: Gate{
				Contains: []string{"showing detailed transcript"},
				Any: []Gate{
					{Contains: []string{"ctrl+o", "to toggle"}},
					{Contains: []string{"ctrl+e", "show all"}},
					{Contains: []string{"ctrl+e", "collapse"}},
					{Contains: []string{"↑↓ scroll"}},
					{Contains: []string{"? for shortcuts"}},
				},
			},
		},
		{
			ID: "live_blocked_form", State: StateBlocked, Priority: 980, Region: "after_last_horizontal_rule",
			VisibleBlocker: true,
			Gate: Gate{
				Contains: []string{"esc to cancel"},
				Any: []Gate{
					{Contains: []string{"enter to confirm"}},
					{Contains: []string{"enter to select"}, Any: []Gate{
						{Contains: []string{"tab/arrow keys to navigate"}},
						{Contains: []string{"arrow keys to navigate"}},
						{Contains: []string{"arrows to navigate"}},
						{Contains: []string{"↑/↓ to navigate"}},
						{Contains: []string{"↑↓ to navigate"}},
					}},
				},
			},
		},
		{
			ID: "dynamic_workflow_prompt", State: StateBlocked, Priority: 980, Region: "whole_recent",
			VisibleBlocker: true,
			Gate:           Gate{Contains: []string{"run a dynamic workflow?", "esc to cancel"}},
		},
		{
			ID: "mcp_elicitation_prompt", State: StateBlocked, Priority: 980, Region: "whole_recent",
			VisibleBlocker: true,
			// MCP elicitation は Accept/Decline と "Esc to cancel" のみで Enter の
			// ヒントが無いため live_blocked_form では拾えない。
			Gate: Gate{
				Contains:  []string{"esc to cancel"},
				LineRegex: []string{`(?i)^\s*MCP server ["\x{201C}].+["\x{201D}] requests your input\s*$`},
				All: []Gate{{Any: []Gate{
					{LineRegex: []string{`^\s*\x{276F}?\s*Accept\b`}},
					{LineRegex: []string{`^\s*\x{276F}?\s*Decline\b`}},
				}}},
			},
		},
		{
			ID: "live_prompt_box", State: StateIdle, Priority: 950, Region: "prompt_box_body",
			VisibleIdle: true,
			Gate: Gate{
				LineRegex: []string{`^\s*❯`},
				Not: []Gate{
					{Contains: []string{"enter to select"}},
					{Contains: []string{"esc to cancel"}},
					{Contains: []string{"tab/arrow keys"}},
					{Contains: []string{"arrow keys to navigate"}},
					{Contains: []string{"↑/↓ to navigate"}},
				},
			},
		},
		{
			ID: "model_picker_menu", State: StateUnknown, Priority: 900, Region: "whole_recent",
			SkipStateUpdate: true,
			Gate: Gate{
				Contains: []string{"select model", "enter to set as default", "esc to cancel"},
				Not: []Gate{
					{Contains: []string{"do you want to proceed?"}},
					{Contains: []string{"enter to select"}},
				},
			},
		},
		{
			ID: "bash_permission_prompt", State: StateBlocked, Priority: 850, Region: "whole_recent",
			VisibleBlocker: true,
			Gate: Gate{
				Contains: []string{"do you want to proceed?"},
				Any: []Gate{
					{Contains: []string{"bash command"}},
					{Contains: []string{"bash("}},
					{Contains: []string{"contains expansion"}},
					{Contains: []string{"tab to amend"}},
					{Contains: []string{"ctrl+e to explain"}},
				},
				// claude は選択中の選択肢に "❯" を付けるので全分岐でその接頭辞を許容する。
				All: []Gate{{Any: []Gate{
					{LineRegex: []string{`(?i)^\s*❯?\s*yes\b`}},
					{LineRegex: []string{`(?i)^\s*❯?\s*1\.\s*yes\b`}},
					{LineRegex: []string{`(?i)^\s*❯?\s*2\.\s*yes\b`}},
					{LineRegex: []string{`(?i)^\s*❯?\s*2\.\s*no\b`}},
					{LineRegex: []string{`(?i)^\s*❯?\s*3\.\s*no\b`}},
				}}},
			},
		},
		{
			ID: "generic_permission_prompt", State: StateBlocked, Priority: 840, Region: "after_last_horizontal_rule",
			VisibleBlocker: true,
			Gate: Gate{
				Contains: []string{"do you want to proceed?", "esc to cancel"},
				All: []Gate{{Any: []Gate{
					{LineRegex: []string{`(?i)^\s*❯?\s*1\.\s*yes\b`}},
					{LineRegex: []string{`(?i)^\s*2\.\s*yes\b`}},
					{LineRegex: []string{`(?i)^\s*2\.\s*no\b`}},
					{LineRegex: []string{`(?i)^\s*3\.\s*no\b`}},
				}}},
			},
		},
		{
			ID: "legacy_no_prompt_blocker", State: StateBlocked, Priority: 300, Region: "whole_recent",
			Gate: Gate{
				Any: []Gate{
					{Contains: []string{"do you want to"}, Any: []Gate{{Contains: []string{"yes"}}, {Contains: []string{"❯"}}}},
					{Contains: []string{"would you like to"}, Any: []Gate{{Contains: []string{"yes"}}, {Contains: []string{"❯"}}}},
					{Contains: []string{"waiting for permission"}},
					{Contains: []string{"do you want to allow this connection?"}},
					{Contains: []string{"tab to amend"}},
					{Contains: []string{"ctrl+e to explain"}},
					{Contains: []string{"do you want to proceed?", "esc to cancel"}},
					{Contains: []string{"review your answers"}},
					{Contains: []string{"skip interview and plan immediately"}},
				},
				Not: []Gate{{Regex: []string{`(?m)^\s*❯\s*$`}}},
			},
		},
		{
			ID: "osc_title_idle", State: StateIdle, Priority: 250, Region: "osc_title",
			VisibleIdle: true,
			Gate:        Gate{Regex: []string{`^\x{2733} `}},
		},
		{
			ID: "osc_progress_idle", State: StateIdle, Priority: 250, Region: "osc_progress",
			Gate: Gate{Regex: []string{`^4;0`}},
		},
	},
}

var codexManifest = Manifest{
	Tool: "codex",
	Rules: []Rule{
		{
			ID: "osc_title_blocked", State: StateBlocked, Priority: 1100, Region: "osc_title",
			VisibleBlocker: true,
			Gate:           Gate{Contains: []string{"Action Required"}},
		},
		{
			ID: "osc_title_working", State: StateWorking, Priority: 1050, Region: "osc_title",
			VisibleWorking: true,
			Gate:           Gate{Regex: []string{`(?:^| )[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏](?: |$)`}},
		},
		{
			ID: "transcript_viewer", State: StateUnknown, Priority: 1000, Region: "after_last_prompt_marker",
			SkipStateUpdate: true,
			Gate: Gate{
				Contains: []string{"↑/↓ to scroll", "pgup/pgdn to", "home/end to jump", "q to quit"},
				Any: []Gate{
					{Contains: []string{"esc to edit prev"}},
					{Contains: []string{"esc/← to edit prev"}},
				},
			},
		},
		{
			ID: "trust_directory", State: StateBlocked, Priority: 950, Region: "top_non_empty_lines(20)",
			VisibleBlocker: true,
			Gate: Gate{All: []Gate{
				{Regex: []string{`\A> You are in [^\r\n]+(?:\r?\n|$)`}},
				{Regex: []string{`(?s)Do\s+you\s+trust\s+the\s+contents\s+of\s+this\s+directory\?`}},
			}},
		},
		{
			ID: "startup_update", State: StateBlocked, Priority: 950, Region: "bottom_non_empty_lines(20)",
			VisibleBlocker: true,
			Gate: Gate{
				Contains: []string{"Update available!", "Update now"},
				Regex:    []string{`Skip\s+until\s+next\s+version`, `Press enter to continue\s*\z`},
			},
		},
		{
			ID: "live_strong_blocker", State: StateBlocked, Priority: 900, Region: "after_last_prompt_marker",
			VisibleBlocker: true,
			Gate: Gate{Any: []Gate{
				{Contains: []string{"press enter to confirm or esc to cancel"}},
				{Contains: []string{"enter to submit answer"}},
				{Contains: []string{"enter to submit all"}},
				{Contains: []string{"allow command?"}},
			}},
		},
		{
			ID: "weak_blocker", State: StateBlocked, Priority: 600, Region: "whole_recent_without_current_prompt_marker",
			Gate: Gate{Any: []Gate{
				{Contains: []string{"[y/n]"}},
				{Contains: []string{"yes (y)"}},
				{Contains: []string{"do you want to"}, Any: []Gate{{Contains: []string{"yes"}}, {Contains: []string{"❯"}}}},
				{Contains: []string{"would you like to"}, Any: []Gate{{Contains: []string{"yes"}}, {Contains: []string{"❯"}}}},
			}},
		},
		{
			ID: "screen_working_fallback", State: StateWorking, Priority: 500, Region: "bottom_non_empty_lines(3)",
			VisibleWorking: true,
			Gate: Gate{
				LineRegex: []string{`^[•◦]\s+Working \([^)]*esc to interrupt\)(?: · .*)?$`},
				Not:       []Gate{{Contains: []string{"■ Conversation interrupted"}}},
			},
		},
		{
			ID: "osc_title_idle", State: StateIdle, Priority: 100, Region: "osc_title",
			VisibleIdle: true,
			Gate: Gate{
				Regex: []string{`\S`},
				Not: []Gate{
					{Regex: []string{`(?:^| )[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏](?: |$)`}},
					{Contains: []string{"Action Required"}},
				},
			},
		},
	},
}

// manifests はツール名 → コンパイル済みマニフェスト。
var manifests = map[string]*CompiledManifest{
	"claude": MustCompile(claudeManifest),
	"codex":  MustCompile(codexManifest),
}

// ManifestFor は tool のマニフェストを返す(無ければ nil)。
func ManifestFor(tool string) *CompiledManifest { return manifests[tool] }
