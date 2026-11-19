package agent

import (
	"strings"
	"testing"
)

func TestSummarizeToolResultSmall(t *testing.T) {
	// Small list_dir with 2 items should NOT be summarized
	smallListDir := `{"path":"C:\\test","entries":[{"name":"a.txt","directory":false},{"name":"b","directory":true}],"shown":2,"truncated":false}`
	res := SummarizeToolResult("list_dir", smallListDir)
	if res != smallListDir {
		t.Fatalf("expected small output to remain verbatim, got: %s", res)
	}

	// Small search result (1 item) should remain verbatim
	smallSearch := `{"path":"C:\\test","query":"doc","entries":[{"name":"doc.txt","directory":false,"full_name":"C:\\test\\doc.txt"}],"shown":1}`
	res = SummarizeToolResult("search_entries", smallSearch)
	if res != smallSearch {
		t.Fatalf("expected small search to remain verbatim, got: %s", res)
	}
}

func TestSummarizeToolResultLargeExploratory(t *testing.T) {
	// Large list_dir with 5 items should be summarized
	largeListDir := `{"path":"C:\\test","entries":[{"name":"1.txt","directory":false},{"name":"2.txt","directory":false},{"name":"3.txt","directory":false},{"name":"4.txt","directory":false},{"name":"5.txt","directory":false}],"shown":5,"truncated":true}`
	res := SummarizeToolResult("list_dir", largeListDir)
	if !strings.Contains(res, "contains 5 items") {
		t.Fatalf("expected summarized entry count, got: %s", res)
	}
	if !strings.Contains(res, "[truncated; more items exist]") {
		t.Fatalf("expected truncated notice, got: %s", res)
	}
	if strings.Contains(res, "5.txt") {
		t.Fatalf("expected 5th item to be omitted from summary, got: %s", res)
	}

	// Large search result with 5 matches should be summarized
	largeSearch := `{"path":"C:\\test","query":"log","entries":[{"name":"1.log","directory":false,"full_name":"C:\\test\\1.log"},{"name":"2.log","directory":false,"full_name":"C:\\test\\2.log"},{"name":"3.log","directory":false,"full_name":"C:\\test\\3.log"},{"name":"4.log","directory":false,"full_name":"C:\\test\\4.log"}],"shown":4}`
	resSearch := SummarizeToolResult("search_entries", largeSearch)
	if !strings.Contains(resSearch, "returned 4 matches") {
		t.Fatalf("expected summarized match count, got: %s", resSearch)
	}
	if !strings.Contains(resSearch, "Top 3:") {
		t.Fatalf("expected top 3 matches, got: %s", resSearch)
	}
}

func TestSummarizeToolResultReadFile(t *testing.T) {
	short := "line 1\nline 2"
	if res := SummarizeToolResult("read_file", short); res != short {
		t.Fatalf("expected short file to remain verbatim, got: %s", res)
	}

	long := strings.Repeat("A very long line of configuration or data text\n", 20)
	res := SummarizeToolResult("read_file", long)
	if !strings.Contains(res, "File content (20 lines") {
		t.Fatalf("expected file summary, got: %s", res)
	}
}

func TestFormatKnownFolders(t *testing.T) {
	formatted := FormatKnownFolders([]KnownFolder{
		{Name: "Desktop", Path: `C:\Users\test\Desktop`},
		{Name: "Downloads", Path: `C:\Users\test\Downloads`},
	})
	if !strings.Contains(formatted, "- Desktop: C:\\Users\\test\\Desktop") {
		t.Fatalf("expected Desktop in formatted folders, got: %s", formatted)
	}
	if !strings.Contains(formatted, "- Downloads: C:\\Users\\test\\Downloads") {
		t.Fatalf("expected Downloads in formatted folders, got: %s", formatted)
	}

	if empty := FormatKnownFolders(nil); !strings.Contains(empty, "discovery") {
		t.Fatalf("expected a discovery fallback for no folders, got: %s", empty)
	}
}

func TestMatchKnownFoldersUsesWholeWords(t *testing.T) {
	folders := []KnownFolder{
		{Name: "Music", Path: `C:\Users\test\Music`},
		{Name: "Downloads", Path: `C:\Users\test\Downloads`},
	}

	if matches := matchKnownFolders("I am a musician", folders); len(matches) != 0 {
		t.Fatalf("expected no partial-word match, got %+v", matches)
	}
	if matches := matchKnownFolders("open my music folder", folders); len(matches) != 1 || matches[0].Name != "Music" {
		t.Fatalf("expected the Music folder to match, got %+v", matches)
	}
	if matches := matchKnownFolders("move Desktop files to Downloads", folders); len(matches) != 1 || matches[0].Name != "Downloads" {
		t.Fatalf("expected the Downloads folder to match, got %+v", matches)
	}
}
