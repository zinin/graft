package plan

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zinin/graft/internal/config"
)

// The texts of the agents were checked with live models: a change of them
// needs that check again.
const (
	solDescription = "General-purpose agent on GPT-6.1 Sol: Codex via CLIProxyAPI. Use it when the user or their " +
		"instructions ask for this model. Call it without `model`: a `model` in the call would replace GPT-6.1 Sol."
	solGuard = "This agent runs on GPT-6.1 Sol and on no other model. Before anything else, look at the model that your " +
		"environment section says you are powered by. If its ID starts with `claude-`, the call that started you replaced " +
		"the model by mistake: use no tool, leave the task undone, and reply with this one line only: " +
		"\"Wrong model: this agent runs on GPT-6.1 Sol. Call it again without `model`.\"\n\nOtherwise, do "
	agentTask = "the task in the message you got with the tools you have. Complete it fully: don't gold-plate, but don't " +
		"leave it half-done. You are the agent for this task: do the work yourself, and do not hand the whole task to " +
		"another agent. When you are done, reply with a concise report of what you did and what you found: the agent that " +
		"started you reads only your reply."
)

// agentsArgs returns the values of --agents in args, in order. Claude Code
// takes the last of them.
func agentsArgs(args []string) []string {
	var values []string
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "--agents" && i+1 < len(args) {
			values = append(values, args[i+1])
		} else if v, ok := strings.CutPrefix(a, "--agents="); ok {
			values = append(values, v)
		}
	}
	return values
}

// agentNames returns the names of the agents of p, sorted.
func agentNames(p *Plan) []string { return slices.Sorted(maps.Keys(p.Agents)) }

// TestAgentsWithTheSubscription: behind the router an agent on each external
// model has the model's graft name, the session's effort and a prompt that
// stops a Claude model put in its place.
func TestAgentsWithTheSubscription(t *testing.T) {
	p := build(t, "mix")
	if want := []string{"glm", "kimi", "sol"}; !reflect.DeepEqual(agentNames(p), want) {
		t.Fatalf("agents %q, want %q", agentNames(p), want)
	}
	want := map[string]any{"description": solDescription, "prompt": solGuard + agentTask, "model": "sol", "effort": "max"}
	if !reflect.DeepEqual(p.Agents["sol"], want) {
		t.Errorf("agent sol =\n%v\nwant\n%v", p.Agents["sol"], want)
	}
	if got := p.Agents["glm"].(map[string]any)["description"].(string); !strings.HasPrefix(got, "General-purpose agent on glm: via ollama. Use it ") {
		t.Errorf("description of glm = %q", got)
	}
	if i := slices.Index(p.Args, "--agents"); i < 2 || p.Args[i-2] != "--settings" || p.Args[i+2] != "--allow-dangerously-skip-permissions" {
		t.Errorf("Args = %q, want --agents after --settings and before the args of the config", p.Args)
	}
	values := agentsArgs(p.Args)
	if len(values) != 1 {
		t.Fatalf("%d --agents in %q, want one", len(values), p.Args)
	}
	var passed map[string]any
	if err := json.Unmarshal([]byte(values[0]), &passed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(passed, p.Agents) {
		t.Errorf("--agents =\n%v\nwant\n%v", passed, p.Agents)
	}
}

// TestAgentsOfADirectProfile: in the direct mode Claude Code sees the ids,
// so an agent has the model's name and its id. Without the subscription a
// model in the call is an external one too, so the prompt has no guard.
func TestAgentsOfADirectProfile(t *testing.T) {
	p := build(t, "pair")
	if want := []string{"glm-proxy", "sol"}; !reflect.DeepEqual(agentNames(p), want) {
		t.Fatalf("agents %q, want %q", agentNames(p), want)
	}
	want := map[string]any{
		"description": solDescription,
		"prompt":      "This agent runs on GPT-6.1 Sol.\n\nDo " + agentTask,
		"model":       "gpt-6.1-sol",
		"effort":      "max",
	}
	if !reflect.DeepEqual(p.Agents["sol"], want) {
		t.Errorf("agent sol =\n%v\nwant\n%v", p.Agents["sol"], want)
	}
	if got := p.Agents["glm-proxy"].(map[string]any)["model"]; got != "glm-5.3-flash" {
		t.Errorf("model of glm-proxy = %v, want its id", got)
	}
}

// TestAgentsBehindTheRouterWithoutTheSubscription: there too a model in the
// call is an external one, so the prompt has no guard.
func TestAgentsBehindTheRouterWithoutTheSubscription(t *testing.T) {
	sol := build(t, "multi").Agents["sol"].(map[string]any)
	if sol["model"] != "sol" || sol["prompt"] != "This agent runs on GPT-6.1 Sol.\n\nDo "+agentTask {
		t.Errorf("agent sol = %v, want its graft name and no guard", sol)
	}
}

// TestAgentsNeedAChoice: a session on one model has no other to hand a task
// to, and a profile on the subscription alone has no external model.
func TestAgentsNeedAChoice(t *testing.T) {
	for _, profile := range []string{"sol", "glm-proxy", "kimi", "tuned", "plain"} {
		p := build(t, profile)
		if p.Agents != nil || agentsArgs(p.Args) != nil {
			t.Errorf("%s: agents %v, Args %q, want none", profile, p.Agents, p.Args)
		}
	}
	// One external model is a choice next to the subscription.
	if got := agentNames(build(t, "sol-plus")); !reflect.DeepEqual(got, []string{"sol"}) {
		t.Errorf("sol-plus: agents %q, want sol", got)
	}
}

// TestAgentsOncePerModel: a model that the profile names as model, alias,
// subagents and in picker is one agent, and a model only other profiles use
// is none.
func TestAgentsOncePerModel(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  a: { provider: p, context: 200000 }
  b: { provider: p, context: 200000 }
  c: { provider: p, context: 200000 }
  other: { provider: p, context: 200000 }
profiles:
  all: { model: a, picker: [a, b], aliases: { haiku: c, opus: a }, subagents: c }
  else: { model: other }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(Input{Config: cfg, Profile: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(agentNames(p), want) {
		t.Errorf("agents %q, want %q", agentNames(p), want)
	}
}

// TestAgentTextsFitTheList: Claude Code lists the agents one per line, so a
// label or a description of several lines becomes one, and the JSON keeps
// the characters as written.
func TestAgentTextsFitTheList(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  m: { provider: p, context: 200000 }
  odd:
    provider: p
    context: 200000
    label: "GLM \"Flash\"\n<beta> & ещё"
    description: |
      fast,
        cheap.
profiles:
  two: { model: m, picker: [odd] }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(Input{Config: cfg, Profile: "two"})
	if err != nil {
		t.Fatal(err)
	}
	const label = `GLM "Flash" <beta> & ещё`
	want := "General-purpose agent on " + label + ": fast, cheap. Use it when the user or their instructions ask for this model. " +
		"Call it without `model`: a `model` in the call would replace " + label + "."
	odd := p.Agents["odd"].(map[string]any)
	if odd["description"] != want {
		t.Errorf("description = %q, want %q", odd["description"], want)
	}
	if got := odd["prompt"].(string); !strings.HasPrefix(got, "This agent runs on "+label+".\n\nDo the task ") {
		t.Errorf("prompt = %q", got)
	}
	if arg := strings.Join(agentsArgs(p.Args), ""); !strings.Contains(arg, `<beta> & ещё: fast, cheap.`) {
		t.Errorf("--agents escapes what it need not: %s", arg)
	}
}

// TestAgentsEffort: an agent runs at the level the session starts at, which
// the last --effort sets. A level that a definition cannot have is left out,
// since Claude Code refuses to start with a definition it cannot read.
func TestAgentsEffort(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  a: { provider: p, context: 200000 }
  b: { provider: p, context: 200000 }
profiles:
  none: { model: a, picker: [b] }
  high: { model: a, picker: [b], effort: high }
  argued: { model: a, picker: [b], effort: high, args: [--effort, low] }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		profile string
		args    []string
		want    any // nil when the definition has no effort
	}{
		{"none", nil, nil},
		{"none", []string{"--effort", "max"}, "max"},
		{"high", nil, "high"},
		{"high", []string{"--effort", "low"}, "low"},
		{"high", []string{"--effort=xhigh", "-p"}, "xhigh"},
		{"high", []string{"--effort", "low", "--effort", "max"}, "max"},
		// Claude Code takes a level in any case, with white space around it
		// and as med.
		{"high", []string{"--effort", "MAX"}, "max"},
		{"high", []string{"--effort= low "}, "low"},
		{"high", []string{"--effort", "med"}, "medium"},
		{"high", []string{"--effort", "ultracode"}, nil},
		{"high", []string{"--effort="}, nil},
		{"high", []string{"-p", "--", "--effort", "low"}, "high"},
		{"argued", nil, "low"},
		{"argued", []string{"--effort", "medium"}, "medium"},
	}
	for _, tt := range tests {
		p, err := Build(Input{Config: cfg, Profile: tt.profile, Args: tt.args})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"a", "b"} {
			if got := p.Agents[name].(map[string]any)["effort"]; got != tt.want {
				t.Errorf("%s %q: effort of %s = %v, want %v", tt.profile, tt.args, name, got, tt.want)
			}
		}
	}
}

// TestOwnAgentsJoin: Claude Code takes the last of several --agents, so
// graft's agents join the user's there, in its place. An agent of the user's
// wins over graft's of the same name, and stays as written.
func TestOwnAgentsJoin(t *testing.T) {
	const (
		first = `{"first":{"description":"x","prompt":""}}`
		mine  = `{"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20},"sol":{"description":"My Sol.","prompt":""}}`
	)
	joined := []string{"glm", "kimi", "mine", "sol"}
	tests := []struct {
		name string
		args []string
		want []string // the names of the agents
		tail []string // the arguments after those of the config, with the joined agents as <agents>
	}{
		{"as two arguments", []string{"--agents", mine, "--resume"}, joined, []string{"--agents", "<agents>", "--resume"}},
		{"as one argument", []string{"-p", "--agents=" + mine, "--", "hi"}, joined, []string{"-p", "--agents=<agents>", "--", "hi"}},
		{"twice", []string{"--agents", first, "--agents", mine}, joined, []string{"--agents", first, "--agents", "<agents>"}},
		{"empty object", []string{"--agents", "{}"}, []string{"glm", "kimi", "sol"}, []string{"--agents", "<agents>"}},
		// An option with several values or an optional one reads on up to the
		// next option: without --agents there it would take the prompt.
		{"after an optional value", []string{"--resume", "--agents", mine, "go on"}, joined,
			[]string{"--resume", "--agents", "<agents>", "go on"}},
		{"after several values", []string{"-p", "--allowedTools", "Read", "Grep", "--agents", mine, "review"}, joined,
			[]string{"-p", "--allowedTools", "Read", "Grep", "--agents", "<agents>", "review"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := build(t, "mix", tt.args...)
			values := agentsArgs(p.Args)
			arg := values[len(values)-1]
			var passed map[string]json.RawMessage
			if err := json.Unmarshal([]byte(arg), &passed); err != nil {
				t.Fatalf("--agents %s: %v", arg, err)
			}
			if got := slices.Sorted(maps.Keys(passed)); !reflect.DeepEqual(got, tt.want) || !reflect.DeepEqual(agentNames(p), tt.want) {
				t.Errorf("agents %q in Args and %q in the plan, want %q", got, agentNames(p), tt.want)
			}
			if slices.Contains(tt.want, "mine") {
				for _, def := range []string{`"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20}`, `"sol":{"description":"My Sol.","prompt":""}`} {
					if !strings.Contains(arg, def) {
						t.Errorf("--agents lacks %s:\n%s", def, arg)
					}
				}
			}
			tail := slices.Clone(p.Args[slices.Index(p.Args, "--settings")+2:])
			for i := range tail {
				tail[i] = strings.Replace(tail[i], arg, "<agents>", 1)
			}
			if want := append([]string{"--allow-dangerously-skip-permissions"}, tt.tail...); !reflect.DeepEqual(tail, want) {
				t.Errorf("after --settings: %q, want %q", tail, want)
			}
		})
	}
}

// TestOwnAgentsInTheConfig: --agents in the args of the config join graft's
// agents too, in their place, unless the command line has others after them:
// Claude Code takes the last.
func TestOwnAgentsInTheConfig(t *testing.T) {
	const (
		kept = `{"kept":{"description":"x","prompt":""}}`
		mine = `{"mine":{"description":"x","prompt":""}}`
	)
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  a: { provider: p, context: 200000 }
  b: { provider: p, context: 200000 }
defaults:
  args: [--add-dir, /srv/notes, --agents, '`+kept+`']
profiles:
  two: { model: a, picker: [b] }
  one: { model: a }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		profile string
		args    []string
		want    []string // the names of the agents
		tail    []string // the arguments after --settings, with the joined agents as <agents>
	}{
		{"two", []string{"fix the bug"}, []string{"a", "b", "kept"},
			[]string{"--add-dir", "/srv/notes", "--agents", "<agents>", "fix the bug"}},
		{"two", []string{"--agents", mine}, []string{"a", "b", "mine"},
			[]string{"--add-dir", "/srv/notes", "--agents", kept, "--agents", "<agents>"}},
		// A profile without agents of graft passes the user's on as they are.
		{"one", nil, nil, []string{"--add-dir", "/srv/notes", "--agents", kept}},
	}
	for _, tt := range tests {
		p, err := Build(Input{Config: cfg, Profile: tt.profile, Args: tt.args})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(agentNames(p), tt.want) {
			t.Errorf("%s %q: agents %q, want %q", tt.profile, tt.args, agentNames(p), tt.want)
		}
		tail := slices.Clone(p.Args[slices.Index(p.Args, "--settings")+2:])
		if values := agentsArgs(tail); p.Agents != nil {
			for i := range tail {
				tail[i] = strings.Replace(tail[i], values[len(values)-1], "<agents>", 1)
			}
		}
		if !reflect.DeepEqual(tail, tt.tail) {
			t.Errorf("%s %q: after --settings %q, want %q", tt.profile, tt.args, tail, tt.tail)
		}
	}
}

// TestOwnAgentsThatAreNoObject: graft cannot join what is no JSON object,
// such as the path of a file, which Claude Code takes with -p, so the
// user's --agents goes to claude alone, as written.
func TestOwnAgentsThatAreNoObject(t *testing.T) {
	for _, value := range []string{"agents.json", "{broken", "null", "[]", `"text"`, ""} {
		for _, args := range [][]string{{"-p", "--agents", value, "hi"}, {"-p", "--agents=" + value, "hi"}} {
			p := build(t, "mix", args...)
			if p.Agents != nil || !reflect.DeepEqual(agentsArgs(p.Args), []string{value}) {
				t.Errorf("%q: agents %v, --agents %q", args, p.Agents, agentsArgs(p.Args))
			}
			if n := len(p.Args); !reflect.DeepEqual(p.Args[n-len(args):], args) {
				t.Errorf("Args = %q, want them to end with %q", p.Args, args)
			}
		}
	}
}

func TestAgentsAfterDoubleDashAreText(t *testing.T) {
	p := build(t, "mix", "-p", "--", "--agents", "{}")
	if n := len(p.Args); !reflect.DeepEqual(p.Args[n-4:], []string{"-p", "--", "--agents", "{}"}) {
		t.Errorf("Args = %q, want the prompt text kept", p.Args)
	}
	if want := []string{"glm", "kimi", "sol"}; !reflect.DeepEqual(agentNames(p), want) {
		t.Errorf("agents %q, want %q", agentNames(p), want)
	}
}

func TestLastOption(t *testing.T) {
	tests := []struct {
		args          []string
		value, prefix string
		at            int
	}{
		{nil, "", "", -1},
		{[]string{"a", "--x", "1", "b"}, "1", "", 2},
		{[]string{"--x=1", "--x", "", "--x=2=3"}, "2=3", "--x=", 3},
		{[]string{"--x=1", "--x", "", "b"}, "", "", 2},
		{[]string{"--xy", "1", "--x"}, "", "", -1},
		{[]string{"--x", "1", "--", "--x", "2"}, "1", "", 1},
	}
	for _, tt := range tests {
		value, prefix, at := lastOption(tt.args, "--x")
		if value != tt.value || prefix != tt.prefix || at != tt.at {
			t.Errorf("lastOption(%q) = %q, %q, %d, want %q, %q, %d", tt.args, value, prefix, at, tt.value, tt.prefix, tt.at)
		}
	}
}
