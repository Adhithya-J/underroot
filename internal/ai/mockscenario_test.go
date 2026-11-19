package ai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Adhithya-J/underroot.git/internal/validator"
)

const mockPromptTemplate = `TASK:
%s

ENVIRONMENT:
Folder: demo
Working Directory: C:\demo
Parent Directory: C:\
Verified Known Folders:
- Desktop: C:\Users\test\Desktop
- Downloads: C:\Users\test\Downloads
- Documents: C:\Users\test\Documents

CONFIRMED DISCOVERY FACTS:
%s

Generate the final script only from confirmed facts. Treat unknown paths as unavailable.`

func scenarioFinal(t *testing.T, task, facts string) *AgentResponse {
	t.Helper()
	prompt := fmt.Sprintf(mockPromptTemplate, task, facts)
	resp, err := NewScenarioEngine().GetFinalScript([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		t.Fatalf("final script for %q: %v", task, err)
	}
	return resp
}

func TestScenarioProducesValidScripts(t *testing.T) {
	tasks := []string{
		"list the files in this folder",
		"show me the folders here",
		"count the files here",
		"show the largest files",
		"create a folder called demo-data",
		"archive all .log files",
		"delete temporary .tmp files",
		"organize files by extension",
		"find the budget report",
		"read notes.txt",
		"open my Downloads folder",
		"move all .pdf files to Documents",
		"rename notes.txt to journal.txt",
		"show disk space",
		"list the top processes",
	}
	for _, task := range tasks {
		resp := scenarioFinal(t, task, "No additional filesystem facts required.")
		if resp == nil || !resp.IsSafe {
			t.Fatalf("%q should be safe, got %+v", task, resp)
		}
		if strings.TrimSpace(resp.Script) == "" {
			t.Fatalf("%q returned an empty script", task)
		}
		if err := validator.Validate(resp.Script); err != nil {
			t.Fatalf("%q produced a blocked script: %v\n%s", task, err, resp.Script)
		}
	}
}

func TestScenarioMarksDangerousRequestsUnsafe(t *testing.T) {
	resp := scenarioFinal(t, `delete C:\Windows`, "No additional filesystem facts required.")
	if resp.IsSafe {
		t.Fatalf("expected an unsafe response, got %+v", resp)
	}
	if resp.Script != "" {
		t.Fatalf("unsafe responses should not contain a script, got %q", resp.Script)
	}
}

func TestScenarioPlansDiscoveryForUnknownFile(t *testing.T) {
	prompt := fmt.Sprintf(mockPromptTemplate, "open report.pdf", "No additional filesystem facts required.")
	plan, err := NewScenarioEngine().GetDiscoveryPlan([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Questions) != 1 {
		t.Fatalf("expected one discovery question, got %+v", plan)
	}
	if !strings.Contains(plan.Questions[0].Question, "[mock:find:report.pdf]") {
		t.Fatalf("expected a find marker, got %q", plan.Questions[0].Question)
	}

	action, err := NewScenarioEngine().GetDiscoveryAction(plan.Questions[0].Question, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if action.ToolCall == nil || action.ToolCall.Name != "search_entries" {
		t.Fatalf("expected a search_entries tool call, got %+v", action)
	}
	if action.ToolCall.Args["path"] != `C:\demo` {
		t.Fatalf("expected the working directory, got %+v", action.ToolCall.Args)
	}
}

func TestScenarioSkipsDiscoveryForKnownFolders(t *testing.T) {
	prompt := fmt.Sprintf(mockPromptTemplate, "list files on my Desktop", "No additional filesystem facts required.")
	plan, err := NewScenarioEngine().GetDiscoveryPlan([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Questions) != 0 {
		t.Fatalf("verified known folders should need no discovery, got %+v", plan)
	}

	resp := scenarioFinal(t, "list files on my Desktop", "No additional filesystem facts required.")
	if !strings.Contains(resp.Script, `C:\Users\test\Desktop`) {
		t.Fatalf("expected the verified Desktop path, got %s", resp.Script)
	}
}

func TestScenarioUsesConfirmedDirectoryFact(t *testing.T) {
	facts := `- mock:finddir:projects: {"path":"C:\\demo","query":"projects","entries":[{"name":"projects","directory":true,"full_name":"C:\\demo\\projects"}],"shown":1}`
	resp := scenarioFinal(t, "list files in the projects folder", facts)
	if !strings.Contains(resp.Script, `C:\demo\projects`) {
		t.Fatalf("expected the confirmed path, got %s", resp.Script)
	}
}

func TestScenarioSimulatesRetry(t *testing.T) {
	first := scenarioFinal(t, "simulate failure", "No additional filesystem facts required.")
	if !strings.Contains(first.Script, "throw") {
		t.Fatalf("expected the first attempt to throw, got %s", first.Script)
	}

	prompt := fmt.Sprintf(mockPromptTemplate, "simulate failure", "No additional filesystem facts required.")
	prompt = strings.Replace(prompt, "CONFIRMED DISCOVERY FACTS:",
		"CURRENT RETRY ERROR:\nError Type: DryRunFailed\nError Message: Simulated failure\n\nCONFIRMED DISCOVERY FACTS:", 1)
	resp, err := NewScenarioEngine().GetFinalScript([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Script, "Recovered after retry") {
		t.Fatalf("expected a recovered script, got %s", resp.Script)
	}
}

func TestScenarioRepairsAfterPermissionDenied(t *testing.T) {
	prompt := fmt.Sprintf(mockPromptTemplate, "archive all .log files", "No additional filesystem facts required.")
	prompt = strings.Replace(prompt, "CONFIRMED DISCOVERY FACTS:",
		"CURRENT RETRY ERROR:\nError Type: PermissionDenied\nError Message: blocked by policy\n\nCONFIRMED DISCOVERY FACTS:", 1)
	resp, err := NewScenarioEngine().GetFinalScript([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsSafe || !strings.Contains(resp.Script, "Get-ChildItem") {
		t.Fatalf("expected a read-only fallback, got %+v", resp)
	}
}

func TestScenarioRenamesExtensions(t *testing.T) {
	resp := scenarioFinal(t, "rename all .txt files to .md", "No additional filesystem facts required.")
	if !strings.Contains(resp.Script, "ChangeExtension") || !strings.Contains(resp.Script, "'.md'") {
		t.Fatalf("expected an extension rename, got %s", resp.Script)
	}
}
