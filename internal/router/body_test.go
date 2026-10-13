package router

import (
	"net/http"
	"reflect"
	"testing"
)

func TestFindModel(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		model string // empty: no model found
		value string // what body[start:end] holds
	}{
		{"plain", `{"model":"glm","max_tokens":5}`, "glm", `"glm"`},
		{"spaces around", `{ "max_tokens" : 5 , "model" : "glm" , "stream":true }`, "glm", `"glm"`},
		{"escaped", `{"model":"gl\u006d"}`, "glm", `"gl\u006d"`},
		{"nested model ignored", `{"metadata":{"model":"x"},"messages":[{"model":"y"}],"model":"glm"}`, "glm", `"glm"`},
		{"last one counts", `{"model":"a","model":"glm"}`, "glm", `"glm"`},
		{"last one not a string", `{"model":"glm","model":7}`, "", ""},
		{"not a string", `{"model":["glm"]}`, "", ""},
		{"no model", `{"messages":[]}`, "", ""},
		{"not an object", `[{"model":"glm"}]`, "", ""},
		{"two values", `{"model":"glm"} {}`, "", ""},
		{"not json", `model=glm`, "", ""},
		{"empty", ``, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, start, end, ok := findModel([]byte(tt.body))
			if ok != (tt.model != "") || model != tt.model {
				t.Fatalf("findModel = %q, %v; want %q", model, ok, tt.model)
			}
			if ok && tt.body[start:end] != tt.value {
				t.Errorf("value at [%d:%d] = %s, want %s", start, end, tt.body[start:end], tt.value)
			}
		})
	}
}

func TestWithModel(t *testing.T) {
	body := `{"max_tokens":5, "model" : "glm" ,"messages":[{"role":"user","content":"model: glm"}]}`
	_, start, end, ok := findModel([]byte(body))
	if !ok {
		t.Fatal("no model")
	}
	got := string(withModel([]byte(body), start, end, "glm-5.3-flash"))
	want := `{"max_tokens":5, "model" : "glm-5.3-flash" ,"messages":[{"role":"user","content":"model: glm"}]}`
	if got != want {
		t.Errorf("withModel =\n%s\nwant\n%s", got, want)
	}
}

func TestDropOAuthBetas(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string // nil: no header
	}{
		{"one oauth value", []string{"claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14"},
			[]string{"claude-code-20250219,interleaved-thinking-2025-05-14"}},
		{"only oauth", []string{"oauth-2025-04-20"}, nil},
		{"two lines", []string{"claude-code-20250219", "oauth-2025-04-20, context-management-2025-06-27"},
			[]string{"claude-code-20250219,context-management-2025-06-27"}},
		{"no oauth left as is", []string{"claude-code-20250219, effort-2025-11-24"},
			[]string{"claude-code-20250219, effort-2025-11-24"}},
		{"no header", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tt.in {
				h.Add("Anthropic-Beta", v)
			}
			dropOAuthBetas(h)
			if got := h.Values("Anthropic-Beta"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("anthropic-beta = %q, want %q", got, tt.want)
			}
		})
	}
}
