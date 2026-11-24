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

// agentsArg returns the value of --agents in args, "" for none. Claude Code
// would take the last of several, so more than one fails the test.
func agentsArg(t *testing.T, args []string) string {
	t.Helper()
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
	if len(values) > 1 {
		t.Fatalf("%d --agents in %q", len(values), args)
	}
	return strings.Join(values, "")
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
	var passed map[string]any
	if err := json.Unmarshal([]byte(agentsArg(t, p.Args)), &passed); err != nil {
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

// TestAgentsNeedAChoice: a session on one model has no other to hand a task
// to, and a profile on the subscription alone has no external model.
func TestAgentsNeedAChoice(t *testing.T) {
	for _, profile := range []string{"sol", "glm-proxy", "kimi", "tuned", "plain"} {
		p := build(t, profile)
		if p.Agents != nil || agentsArg(t, p.Args) != "" {
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
	if arg := agentsArg(t, p.Args); !strings.Contains(arg, `<beta> & ещё: fast, cheap.`) {
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
// graft passes one with its agents and the user's. An agent of the user's
// wins over graft's of the same name, and stays as written.
func TestOwnAgentsJoin(t *testing.T) {
	const mine = `{"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20},"sol":{"description":"My Sol.","prompt":""}}`
	tests := []struct {
		name  string
		args  []string
		want  []string // the names of the agents
		after []string // what follows --agents in Args
	}{
		{"as two arguments", []string{"--agents", mine, "--resume"}, []string{"glm", "kimi", "mine", "sol"}, []string{"--resume"}},
		{"as one argument", []string{"-p", "--agents=" + mine, "--", "hi"}, []string{"glm", "kimi", "mine", "sol"}, []string{"-p", "--", "hi"}},
		{"twice", []string{"--agents", `{"first":{"description":"x","prompt":""}}`, "--agents", mine}, []string{"glm", "kimi", "mine", "sol"}, nil},
		{"empty object", []string{"--agents", "{}"}, []string{"glm", "kimi", "sol"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := build(t, "mix", tt.args...)
			if !reflect.DeepEqual(agentNames(p), tt.want) {
				t.Fatalf("agents %q, want %q", agentNames(p), tt.want)
			}
			arg := agentsArg(t, p.Args)
			if len(tt.want) == 4 {
				for _, def := range []string{`"mine":{"description":"Mine.","prompt":"a<b","maxTurns":20}`, `"sol":{"description":"My Sol.","prompt":""}`} {
					if !strings.Contains(arg, def) {
						t.Errorf("--agents lacks %s:\n%s", def, arg)
					}
				}
			}
			i := slices.Index(p.Args, "--agents")
			want := append([]string{"--allow-dangerously-skip-permissions"}, tt.after...)
			if got := p.Args[i+2:]; !reflect.DeepEqual(got, want) {
				t.Errorf("after --agents: %q, want %q", got, want)
			}
		})
	}
}

// TestOwnAgentsInTheConfig: --agents in the args of a profile join graft's
// agents too, and one on the command line counts instead of it, as the last.
func TestOwnAgentsInTheConfig(t *testing.T) {
	cfg, err := config.Parse([]byte(`
providers:
  p: { url: "https://example.com", key: { env: P_KEY } }
models:
  a: { provider: p, context: 200000 }
  b: { provider: p, context: 200000 }
defaults:
  args: [--verbose, --agents, '{"kept":{"description":"x","prompt":""}}']
profiles:
  two: { model: a, picker: [b] }
  one: { model: a }
`), filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	names := func(profile string, args ...string) ([]string, []string) {
		t.Helper()
		p, err := Build(Input{Config: cfg, Profile: profile, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		return agentNames(p), p.Args
	}
	if got, _ := names("two"); !reflect.DeepEqual(got, []string{"a", "b", "kept"}) {
		t.Errorf("agents %q, want a, b and kept", got)
	}
	if got, _ := names("two", "--agents", `{"mine":{"description":"x","prompt":""}}`); !reflect.DeepEqual(got, []string{"a", "b", "mine"}) {
		t.Errorf("agents %q, want a, b and mine", got)
	}
	// A profile without agents of graft passes the user's on as they are.
	got, args := names("one")
	if got != nil || !strings.HasPrefix(agentsArg(t, args), `{"kept":`) {
		t.Errorf("agents %q, Args %q", got, args)
	}
}

// TestOwnAgentsThatAreNoObject: graft cannot join what is no JSON object,
// such as the path of a file, which Claude Code takes with -p, so the
// user's --agents goes to claude alone, as written.
func TestOwnAgentsThatAreNoObject(t *testing.T) {
	for _, value := range []string{"agents.json", "{broken", "null", "[]", `"text"`, ""} {
		for _, args := range [][]string{{"-p", "--agents", value, "hi"}, {"-p", "--agents=" + value, "hi"}} {
			p := build(t, "mix", args...)
			if p.Agents != nil || agentsArg(t, p.Args) != value {
				t.Errorf("%q: agents %v, --agents %q", args, p.Agents, agentsArg(t, p.Args))
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

func TestTakeOption(t *testing.T) {
	tests := []struct {
		args, rest, values []string
	}{
		{nil, nil, nil},
		{[]string{"a", "--x", "1", "b"}, []string{"a", "b"}, []string{"1"}},
		{[]string{"--x=1", "--x", "", "--x=2=3"}, nil, []string{"1", "", "2=3"}},
		{[]string{"--xy", "1", "--x"}, []string{"--xy", "1", "--x"}, nil},
		{[]string{"--x", "1", "--", "--x", "2"}, []string{"--", "--x", "2"}, []string{"1"}},
	}
	for _, tt := range tests {
		args := slices.Clone(tt.args)
		rest, values := takeOption(args, "--x")
		if !reflect.DeepEqual(rest, tt.rest) || !reflect.DeepEqual(values, tt.values) {
			t.Errorf("takeOption(%q) = %q, %q, want %q, %q", tt.args, rest, values, tt.rest, tt.values)
		}
		if !reflect.DeepEqual(args, tt.args) {
			t.Errorf("takeOption changed its argument %q to %q", tt.args, args)
		}
	}
}
