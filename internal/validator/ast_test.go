package validator

import (
	"strings"
	"testing"

	"github.com/Adhithya-J/underroot.git/internal/executor"
)

func TestValidateAnalysisRejectsParseErrors(t *testing.T) {
	err := ValidateAnalysis(&executor.ScriptAnalysis{
		ParseErrors: []string{"Missing closing '}' in statement block."},
	})
	if err == nil || !strings.Contains(err.Error(), "parser reported errors") {
		t.Fatalf("expected a parse error, got %v", err)
	}
}

func TestValidateAnalysisRejectsDangerousAlias(t *testing.T) {
	for _, name := range []string{"iex", "Invoke-Expression", "iwr", "Invoke-WebRequest"} {
		err := ValidateAnalysis(&executor.ScriptAnalysis{
			Commands: []executor.ASTCommand{{Name: name}},
		})
		if err == nil {
			t.Fatalf("expected %q to be blocked", name)
		}
	}
}

func TestValidateAnalysisRejectsEncodedCommand(t *testing.T) {
	err := ValidateAnalysis(&executor.ScriptAnalysis{
		Commands: []executor.ASTCommand{{
			Name:       "powershell",
			Parameters: []string{"NoProfile", "EncodedCommand"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "EncodedCommand") {
		t.Fatalf("expected encoded command rejection, got %v", err)
	}
}

func TestValidateAnalysisRejectsRunAs(t *testing.T) {
	err := ValidateAnalysis(&executor.ScriptAnalysis{
		Commands: []executor.ASTCommand{{
			Name:       "Start-Process",
			Parameters: []string{"Verb"},
			Arguments:  []string{"notepad.exe", "RunAs"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "privilege escalation") {
		t.Fatalf("expected RunAs rejection, got %v", err)
	}
}

func TestValidateAnalysisRejectsDownloadMember(t *testing.T) {
	err := ValidateAnalysis(&executor.ScriptAnalysis{
		Members: []executor.ASTMember{{Target: "$client", Member: "DownloadString"}},
	})
	if err == nil || !strings.Contains(err.Error(), "downloads remote content") {
		t.Fatalf("expected download rejection, got %v", err)
	}
}

func TestValidateAnalysisAcceptsSafeListing(t *testing.T) {
	err := ValidateAnalysis(&executor.ScriptAnalysis{
		Commands: []executor.ASTCommand{{
			Name:       "Get-ChildItem",
			Parameters: []string{"Path"},
			Arguments:  []string{"'C:\\tmp'"},
		}},
	})
	if err != nil {
		t.Fatalf("expected safe listing to pass, got %v", err)
	}
}
