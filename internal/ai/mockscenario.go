package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ScenarioEngine is a deterministic, offline stand-in for a chat model. It
// recognizes common filesystem requests, plans a focused discovery call when a
// path is not already known, and returns PowerShell that is safe to dry-run.
//
// The same engine backs MockProvider (in-process) and cmd/mockllm (an
// OpenAI-compatible HTTP server), so both offline paths behave identically.
type ScenarioEngine struct{}

// NewScenarioEngine returns a stateless scenario engine.
func NewScenarioEngine() *ScenarioEngine { return &ScenarioEngine{} }

// GetDiscoveryPlan returns at most one discovery question for requests that
// need a path confirmed by the filesystem.
func (s *ScenarioEngine) GetDiscoveryPlan(input []Message) (*DiscoveryPlan, error) {
	text := lastUserContent(input)
	task := extractMockTask(text)
	if strings.TrimSpace(task) == "" {
		return &DiscoveryPlan{}, nil
	}
	env := parseMockEnv(text)

	switch classifyMockTask(task) {
	case mockUnsafe, mockRetryDemo, mockDisk, mockProcess:
		return &DiscoveryPlan{}, nil
	}

	// A verified known folder named in the task is already a trusted path.
	if len(env.folderMentions(task)) > 0 {
		return &DiscoveryPlan{}, nil
	}

	switch classifyMockTask(task) {
	case mockArchive, mockDelete, mockMove:
		if ext := requestedExtension(task); ext != "" {
			return mockPlanQuestion("mock:findext:"+ext,
				fmt.Sprintf("Which %s files are in the working directory?", ext)), nil
		}
	case mockRead, mockOpen:
		if name := extractFileName(task); name != "" {
			return mockPlanQuestion("mock:find:"+name,
				fmt.Sprintf("Where is %s?", name)), nil
		}
	case mockRename:
		if src, _, ok := parseRename(task); ok {
			return mockPlanQuestion("mock:find:"+src,
				fmt.Sprintf("Where is %s?", src)), nil
		}
	case mockList, mockFind, mockCount, mockLargest:
		if hint := folderHint(task); hint != "" {
			return mockPlanQuestion("mock:finddir:"+hint,
				fmt.Sprintf("Where is the %s folder?", hint)), nil
		}
	}
	return &DiscoveryPlan{}, nil
}

// GetDiscoveryAction turns a marker embedded in the question into one focused
// filesystem tool call. Unknown questions finish discovery immediately.
func (s *ScenarioEngine) GetDiscoveryAction(question, state string) (*DiscoveryAction, error) {
	env := parseMockEnv(state)
	marker := mockMarker(question)
	if marker == nil {
		return &DiscoveryAction{Answer: "No additional discovery needed.", Done: true}, nil
	}

	kind, arg := marker[0], marker[1]
	dir := env.workingDir
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}

	switch kind {
	case "find":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "search_entries",
			Args: map[string]interface{}{
				"path": dir, "query": arg, "type": "any", "limit": float64(10),
			},
		}}, nil
	case "findext":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "find_file",
			Args: map[string]interface{}{
				"path": dir, "extension": arg, "limit": float64(20),
			},
		}}, nil
	case "finddir":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "search_entries",
			Args: map[string]interface{}{
				"path": dir, "query": arg, "type": "directory", "limit": float64(10),
			},
		}}, nil
	case "list":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "list_dir", Args: map[string]interface{}{"path": arg},
		}}, nil
	case "exists":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "exists", Args: map[string]interface{}{"path": arg},
		}}, nil
	case "downloads":
		return &DiscoveryAction{ToolCall: &ToolCall{
			Name: "user_downloads", Args: map[string]interface{}{},
		}}, nil
	}
	return &DiscoveryAction{Answer: "No additional discovery needed.", Done: true}, nil
}

// GetFinalScript returns a plausible PowerShell script for the request.
func (s *ScenarioEngine) GetFinalScript(input []Message) (*AgentResponse, error) {
	if len(input) == 0 {
		return nil, errors.New("cannot generate a mock response without messages")
	}
	text := lastUserContent(input)
	task := extractMockTask(text)
	env := parseMockEnv(text)
	facts := parseMockFacts(text)
	activeError := extractActiveError(text)
	return buildMockFinal(task, env, facts, activeError), nil
}

// ---------------------------------------------------------------------------
// Prompt parsing
// ---------------------------------------------------------------------------

func lastUserContent(input []Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == "user" || input[i].Role == "" {
			return input[i].Content
		}
	}
	if len(input) > 0 {
		return input[len(input)-1].Content
	}
	return ""
}

func extractMockTask(text string) string {
	const marker = "TASK:\n"
	i := strings.Index(text, marker)
	if i < 0 {
		return ""
	}
	rest := text[i+len(marker):]
	if j := strings.Index(rest, "\n\nENVIRONMENT:"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

type mockEnv struct {
	folder       string
	workingDir   string
	parentDir    string
	knownFolders map[string]string
	knownNames   []string
}

func parseMockEnv(text string) mockEnv {
	env := mockEnv{knownFolders: map[string]string{}}
	env.folder = mockLineValue(text, "Folder:")
	env.workingDir = mockLineValue(text, "Working Directory:")
	env.parentDir = mockLineValue(text, "Parent Directory:")

	const marker = "Verified Known Folders:"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return env
	}
	rest := text[idx+len(marker):]
	if nl := strings.Index(rest, "\n"); nl >= 0 {
		rest = rest[nl+1:]
	}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			if len(env.knownNames) > 0 {
				break
			}
			continue
		}
		entry := strings.TrimPrefix(line, "- ")
		name, path, ok := strings.Cut(entry, ": ")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			continue
		}
		env.knownFolders[strings.ToLower(name)] = path
		env.knownNames = append(env.knownNames, name)
	}
	return env
}

func mockLineValue(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

type mockFact struct {
	ID      string
	Summary string
}

func parseMockFacts(text string) []mockFact {
	const marker = "CONFIRMED DISCOVERY FACTS:\n"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return nil
	}
	rest := text[idx+len(marker):]
	for _, stop := range []string{"\n\nGenerate the final script", "\n\nOBJECTIVE", "\n\n## "} {
		if j := strings.Index(rest, stop); j >= 0 {
			rest = rest[:j]
		}
	}

	var facts []mockFact
	var current *mockFact
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(line, "- ") {
			id, summary, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
			if !ok {
				continue
			}
			facts = append(facts, mockFact{ID: id, Summary: summary})
			current = &facts[len(facts)-1]
			continue
		}
		if current != nil && strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			current.Summary += "\n" + strings.TrimSpace(line)
		}
	}
	return facts
}

func extractActiveError(text string) string {
	const marker = "CURRENT RETRY ERROR:\n"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(marker):]
	for _, stop := range []string{"\n\nCONFIRMED DISCOVERY FACTS:", "\nCONFIRMED DISCOVERY FACTS:"} {
		if j := strings.Index(rest, stop); j >= 0 {
			rest = rest[:j]
		}
	}
	return strings.TrimSpace(rest)
}

func mockPlanQuestion(marker, question string) *DiscoveryPlan {
	return &DiscoveryPlan{Questions: []DiscoveryQuestion{{
		ID:       marker,
		Question: fmt.Sprintf("%s [%s]", question, marker),
		Required: true,
	}}}
}

var mockMarkerRe = regexp.MustCompile(`\[mock:([a-z]+):([^\]]+)\]`)

func mockMarker(question string) []string {
	m := mockMarkerRe.FindStringSubmatch(question)
	if len(m) != 3 {
		return nil
	}
	return m[1:]
}

// ---------------------------------------------------------------------------
// Intent recognition
// ---------------------------------------------------------------------------

type mockIntent int

const (
	mockList mockIntent = iota
	mockFind
	mockArchive
	mockDelete
	mockCreate
	mockCount
	mockLargest
	mockRead
	mockOpen
	mockMove
	mockOrganize
	mockRename
	mockDisk
	mockProcess
	mockUnsafe
	mockRetryDemo
)

func classifyMockTask(task string) mockIntent {
	t := strings.ToLower(task)
	switch {
	case isMockUnsafeTask(t):
		return mockUnsafe
	case containsAny(t, "simulate failure", "simulate a failure", "simulate retry",
		"test retry", "test the retry", "retry demo", "self-heal", "fail first"):
		return mockRetryDemo
	case containsAny(t, "organize", "sort files", "group files"):
		return mockOrganize
	case containsAny(t, "rename"):
		return mockRename
	case containsAny(t, "archive", "zip", "compress"):
		return mockArchive
	case containsAny(t, "delete", "remove", "clean up", "cleanup"):
		return mockDelete
	case containsAny(t, "create folder", "create a folder", "create directory",
		"create a directory", "make a folder", "make directory", "make a directory", "new folder"):
		return mockCreate
	case containsAny(t, "disk space", "free space", "drive space", "disk usage"):
		return mockDisk
	case containsAny(t, "how many", "count"):
		return mockCount
	case containsAny(t, "largest", "biggest", "top 10 files", "largest files"):
		return mockLargest
	case containsAny(t, "process", "cpu", "memory usage", "ram "):
		return mockProcess
	case strings.HasPrefix(t, "read ") || containsAny(t, "read the file", "show contents",
		"show the contents", "cat "):
		return mockRead
	case strings.HasPrefix(t, "open ") || containsAny(t, "open the", "open my", "launch"):
		return mockOpen
	case containsAny(t, "move ", "move all", "move the"):
		return mockMove
	case containsAny(t, "find ", "search ", "locate ", "where is"):
		return mockFind
	default:
		return mockList
	}
}

func isMockUnsafeTask(t string) bool {
	return containsAny(t,
		`delete c:\windows`, `delete c:/windows`, `remove c:\windows`, `remove c:/windows`,
		"format c:", "format the disk", "wipe the disk", "delete everything",
		"disable defender", "disable windows defender", "turn off defender",
		"invoke-expression", "download and run", "download and execute",
		"registry run key", "ransomware", "exfiltrate", "credential dump",
	)
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Task facts
// ---------------------------------------------------------------------------

var mockExtRe = regexp.MustCompile(`(?i)\.([a-z0-9]{1,8})\b`)

func requestedExtension(task string) string {
	if m := mockExtRe.FindStringSubmatch(task); len(m) == 2 {
		return "." + strings.ToLower(m[1])
	}
	lower := strings.ToLower(task)
	switch {
	case containsAny(lower, "temp", "tmp"):
		return ".tmp"
	case containsAny(lower, "log"):
		return ".log"
	case containsAny(lower, "backup", "bak"):
		return ".bak"
	case containsAny(lower, "image", "photo", "picture"):
		return ".jpg"
	case containsAny(lower, "pdf"):
		return ".pdf"
	case containsAny(lower, "document", "word"):
		return ".docx"
	case containsAny(lower, "text"):
		return ".txt"
	}
	return ""
}

func extractFileName(task string) string {
	for _, field := range strings.Fields(task) {
		candidate := strings.Trim(field, "\"'`,;:()[]{}")
		if candidate == "" || strings.HasPrefix(candidate, ".") {
			continue
		}
		if strings.ContainsAny(candidate, `\/`) {
			candidate = filepath.Base(candidate)
		}
		ext := strings.TrimPrefix(filepath.Ext(candidate), ".")
		if ext == "" || len(ext) > 8 {
			continue
		}
		base := strings.TrimSuffix(candidate, filepath.Ext(candidate))
		if base == "" {
			continue
		}
		return candidate
	}
	return ""
}

var mockRenameRe = regexp.MustCompile(`(?i)\brename\s+["']?([^"']+?)["']?\s+to\s+["']?([^"']+?)["']?\s*$`)

func parseRename(task string) (src, dst string, ok bool) {
	m := mockRenameRe.FindStringSubmatch(strings.TrimSpace(task))
	if len(m) != 3 {
		return "", "", false
	}
	src = strings.TrimSpace(m[1])
	dst = strings.TrimSpace(m[2])
	return src, dst, src != "" && dst != ""
}

var mockCreateRe = regexp.MustCompile(`(?i)(?:create|make)\s+(?:a\s+|an\s+)?(?:new\s+)?(?:folder|directory)\s+(?:called\s+|named\s+)?["']?([^"']+?)["']?(?:\s+(?:in|under|inside|on|at)\b.*)?$`)

func createFolderName(task string) string {
	m := mockCreateRe.FindStringSubmatch(strings.TrimSpace(task))
	if len(m) != 2 {
		return "NewFolder"
	}
	name := strings.TrimSpace(m[1])
	for _, prefix := range []string{"for ", "to store "} {
		if len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix) {
			name = strings.TrimSpace(name[len(prefix):])
		}
	}
	name = strings.Trim(name, " .,;:")
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) || name == "." || name == ".." {
		return "NewFolder"
	}
	return name
}

var mockFolderHintRe = regexp.MustCompile(`(?i)([a-z0-9][a-z0-9._-]*(?:\s+[a-z0-9][a-z0-9._-]*){0,3})\s+(?:folder|directory)s?\b`)

var mockStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "my": true, "this": true, "that": true,
	"all": true, "any": true, "files": true, "file": true, "in": true,
	"inside": true, "under": true, "from": true, "of": true, "list": true,
	"show": true, "find": true, "search": true, "count": true, "largest": true,
	"biggest": true, "organize": true, "move": true, "delete": true,
	"archive": true, "open": true, "read": true, "create": true, "make": true,
	"new": true, "working": true, "current": true, "parent": true, "home": true,
}

func folderHint(task string) string {
	for _, m := range mockFolderHintRe.FindAllStringSubmatch(task, -1) {
		words := strings.Fields(strings.ToLower(m[1]))
		for len(words) > 0 && mockStopWords[words[0]] {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		candidate := strings.Join(words, " ")
		if candidate == "" || mockStopWords[candidate] {
			continue
		}
		return candidate
	}
	return ""
}

func searchQuery(task string) string {
	if name := extractFileName(task); name != "" {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	if m := regexp.MustCompile(`["']([^"']+)["']`).FindStringSubmatch(task); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	lower := strings.ToLower(task)
	for _, kw := range []string{"find ", "search for ", "search ", "locate ", "where is "} {
		i := strings.Index(lower, kw)
		if i < 0 {
			continue
		}
		q := strings.TrimSpace(task[i+len(kw):])
		for {
			trimmed := false
			for _, p := range []string{"the ", "a ", "an ", "all ", "any ", "files ", "file ", "named ", "called "} {
				if len(q) >= len(p) && strings.EqualFold(q[:len(p)], p) {
					q = strings.TrimSpace(q[len(p):])
					trimmed = true
				}
			}
			if !trimmed {
				break
			}
		}
		q = strings.TrimSuffix(q, " files")
		q = strings.TrimSuffix(q, " file")
		q = strings.TrimSpace(q)
		if q != "" && len(q) < 60 {
			return q
		}
	}
	return "*"
}

func listFilters(task string) (filesOnly, dirsOnly bool) {
	lower := strings.ToLower(task)
	dirsOnly = strings.Contains(lower, "folder") || strings.Contains(lower, "director")
	filesOnly = strings.Contains(lower, "file") && !dirsOnly
	return filesOnly, dirsOnly
}

// ---------------------------------------------------------------------------
// Folder resolution
// ---------------------------------------------------------------------------

type mockFolderMention struct {
	Name  string
	Path  string
	Start int
	End   int
}

func (e mockEnv) folderMentions(task string) []mockFolderMention {
	lower := strings.ToLower(task)
	var mentions []mockFolderMention
	for _, name := range e.knownNames {
		path := e.knownFolders[strings.ToLower(name)]
		needle := strings.ToLower(name)
		offset := 0
		for {
			i := strings.Index(lower[offset:], needle)
			if i < 0 {
				break
			}
			i += offset
			end := i + len(needle)
			if mockWordBoundary(lower, i, end) {
				mentions = append(mentions, mockFolderMention{
					Name: name, Path: path, Start: i, End: end,
				})
			}
			offset = end
		}
	}
	sort.Slice(mentions, func(i, j int) bool { return mentions[i].Start < mentions[j].Start })
	return mentions
}

func mockWordBoundary(text string, start, end int) bool {
	if start > 0 {
		c := text[start-1]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			return false
		}
	}
	if end < len(text) {
		c := text[end]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func firstFactPath(facts []mockFact, idPrefix string) string {
	for _, fact := range facts {
		if !strings.HasPrefix(fact.ID, idPrefix) {
			continue
		}
		if path := firstFullName(fact.Summary); path != "" {
			return path
		}
	}
	return ""
}

func hasFact(facts []mockFact, idPrefix string) bool {
	for _, fact := range facts {
		if strings.HasPrefix(fact.ID, idPrefix) {
			return true
		}
	}
	return false
}

func firstFullName(summary string) string {
	summary = strings.TrimSpace(summary)
	var payload struct {
		Entries []struct {
			FullName string `json:"full_name"`
		} `json:"entries"`
		Files []struct {
			FullName string `json:"full_name"`
		} `json:"files"`
	}
	if strings.HasPrefix(summary, "{") && json.Unmarshal([]byte(summary), &payload) == nil {
		if len(payload.Entries) > 0 {
			return payload.Entries[0].FullName
		}
		if len(payload.Files) > 0 {
			return payload.Files[0].FullName
		}
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`Top 3: \[(.*?)\]`),
		regexp.MustCompile(`First 3: \[(.*?)\]`),
	} {
		if m := re.FindStringSubmatch(summary); len(m) == 2 {
			return strings.TrimSpace(strings.Split(m[1], ", ")[0])
		}
	}
	return ""
}

func resolveMockDirectory(task string, env mockEnv, facts []mockFact, honorHint bool) (string, bool) {
	if mentions := env.folderMentions(task); len(mentions) > 0 {
		return mentions[0].Path, true
	}
	if p := firstFactPath(facts, "mock:finddir:"); p != "" {
		return p, true
	}
	if honorHint && hasFact(facts, "mock:finddir:") {
		return "", false
	}
	if strings.TrimSpace(env.workingDir) == "" {
		return ".", true
	}
	return env.workingDir, true
}

func resolveMockFilePath(task string, env mockEnv, facts []mockFact) (string, bool) {
	if p := firstFactPath(facts, "mock:find:"); p != "" {
		return p, true
	}
	if hasFact(facts, "mock:find:") {
		return "", false
	}
	name := extractFileName(task)
	if name == "" {
		return "", false
	}
	if mentions := env.folderMentions(task); len(mentions) > 0 {
		return filepath.Join(mentions[0].Path, name), true
	}
	if strings.TrimSpace(env.workingDir) == "" {
		return "", false
	}
	return filepath.Join(env.workingDir, name), true
}

func resolveMockOpenPath(task string, env mockEnv, facts []mockFact) (string, bool) {
	if p := firstFactPath(facts, "mock:find:"); p != "" {
		return p, true
	}
	if hasFact(facts, "mock:find:") {
		return "", false
	}
	name := extractFileName(task)
	if name == "" {
		if mentions := env.folderMentions(task); len(mentions) > 0 {
			return mentions[0].Path, true
		}
		if strings.TrimSpace(env.workingDir) == "" {
			return ".", true
		}
		return env.workingDir, true
	}
	if mentions := env.folderMentions(task); len(mentions) > 0 {
		return filepath.Join(mentions[0].Path, name), true
	}
	if strings.TrimSpace(env.workingDir) == "" {
		return name, true
	}
	return filepath.Join(env.workingDir, name), true
}

func resolveMockMove(task string, env mockEnv) (source, dest, ext string, ok bool) {
	mentions := env.folderMentions(task)
	lower := strings.ToLower(task)
	toPos := strings.Index(lower, " to ")
	if toPos < 0 {
		toPos = strings.Index(lower, " into ")
	}

	var sourceMention, destMention *mockFolderMention
	for i := range mentions {
		m := &mentions[i]
		if toPos < 0 {
			continue
		}
		if m.Start > toPos {
			destMention = m
		} else if m.Start < toPos {
			sourceMention = m
		}
	}
	if destMention == nil && len(mentions) > 0 {
		destMention = &mentions[len(mentions)-1]
		if sourceMention == nil && len(mentions) > 1 {
			sourceMention = &mentions[0]
		}
	}
	if destMention == nil {
		return "", "", "", false
	}
	source = env.workingDir
	if sourceMention != nil {
		source = sourceMention.Path
	}
	return source, destMention.Path, requestedExtension(task), true
}

// ---------------------------------------------------------------------------
// Response construction
// ---------------------------------------------------------------------------

func buildMockFinal(task string, env mockEnv, facts []mockFact, activeError string) *AgentResponse {
	intent := classifyMockTask(task)

	if intent == mockUnsafe {
		return &AgentResponse{
			Explanation: "This request is destructive or high-risk, so no script was generated. Ask for a narrower, reversible operation.",
			IsSafe:      false,
		}
	}

	if intent == mockRetryDemo {
		if strings.TrimSpace(activeError) == "" {
			return &AgentResponse{
				Script:      "throw 'Simulated failure: this request is designed to test the retry loop.'",
				Explanation: "Intentionally failing the first attempt so the self-healing loop has something to repair.",
				IsSafe:      true,
			}
		}
		return &AgentResponse{
			Script:      "Write-Output 'Recovered after retry: the simulated failure was handled.'",
			Explanation: "Retry succeeded after the simulated failure was classified and fed back into context.",
			IsSafe:      true,
		}
	}

	if strings.TrimSpace(activeError) != "" {
		if repaired := repairMockResponse(intent, task, env, facts, activeError); repaired != nil {
			return repaired
		}
	}

	switch intent {
	case mockArchive:
		dir, ok := resolveMockDirectory(task, env, facts, false)
		if !ok {
			return mockUnsafePath("I could not confirm the folder that should be archived.")
		}
		ext := requestedExtension(task)
		if ext == "" {
			ext = ".log"
		}
		return &AgentResponse{
			Script:      archiveMockScript(dir, ext),
			Explanation: fmt.Sprintf("Archives %s files in %s into a timestamped zip.", ext, dir),
			IsSafe:      true,
		}

	case mockDelete:
		dir, ok := resolveMockDirectory(task, env, facts, false)
		if !ok {
			return mockUnsafePath("I could not confirm the folder that should be cleaned.")
		}
		ext := requestedExtension(task)
		if ext == "" {
			ext = ".tmp"
		}
		return &AgentResponse{
			Script:      deleteMockScript(dir, ext),
			Explanation: fmt.Sprintf("Removes %s files from %s after an approval prompt.", ext, dir),
			IsSafe:      true,
		}

	case mockCreate:
		base, _ := resolveMockDirectory(task, env, facts, false)
		name := createFolderName(task)
		return &AgentResponse{
			Script:      createMockScript(base, name),
			Explanation: fmt.Sprintf("Creates the folder %s inside %s if it does not already exist.", name, base),
			IsSafe:      true,
		}

	case mockCount:
		dir, ok := resolveMockDirectory(task, env, facts, true)
		if !ok {
			return mockUnsafePath("I could not confirm the folder to count.")
		}
		return &AgentResponse{
			Script:      countMockScript(dir),
			Explanation: fmt.Sprintf("Counts files in %s and groups them by extension.", dir),
			IsSafe:      true,
		}

	case mockLargest:
		dir, ok := resolveMockDirectory(task, env, facts, true)
		if !ok {
			return mockUnsafePath("I could not confirm the folder to inspect.")
		}
		return &AgentResponse{
			Script:      largestMockScript(dir),
			Explanation: fmt.Sprintf("Shows the ten largest files directly in %s.", dir),
			IsSafe:      true,
		}

	case mockRead:
		path, ok := resolveMockFilePath(task, env, facts)
		if !ok {
			return mockUnsafePath("I could not confirm which file to read.")
		}
		return &AgentResponse{
			Script:      readMockScript(path),
			Explanation: fmt.Sprintf("Reads the first 50 lines of %s.", path),
			IsSafe:      true,
		}

	case mockOpen:
		path, ok := resolveMockOpenPath(task, env, facts)
		if !ok {
			return mockUnsafePath("I could not confirm what to open.")
		}
		return &AgentResponse{
			Script:      openMockScript(path),
			Explanation: fmt.Sprintf("Opens %s with its default application.", path),
			IsSafe:      true,
		}

	case mockMove:
		source, dest, ext, ok := resolveMockMove(task, env)
		if !ok {
			return mockUnsafePath("I could not confirm the destination folder for the move.")
		}
		return &AgentResponse{
			Script:      moveMockScript(source, dest, ext),
			Explanation: fmt.Sprintf("Moves matching files from %s to %s.", source, dest),
			IsSafe:      true,
		}

	case mockOrganize:
		dir, _ := resolveMockDirectory(task, env, facts, false)
		return &AgentResponse{
			Script:      organizeMockScript(dir),
			Explanation: fmt.Sprintf("Groups files in %s into subfolders named after their extension.", dir),
			IsSafe:      true,
		}

	case mockRename:
		src, dst, ok := parseRename(task)
		if ok && strings.HasPrefix(dst, ".") {
			dir, _ := resolveMockDirectory(task, env, facts, false)
			srcExt := requestedExtension(src)
			if srcExt == "" {
				srcExt = requestedExtension(task)
			}
			if srcExt != "" {
				return &AgentResponse{
					Script:      renameExtMockScript(dir, srcExt, dst),
					Explanation: fmt.Sprintf("Renames %s files in %s to use the %s extension.", srcExt, dir, dst),
					IsSafe:      true,
				}
			}
		}
		path, ok := resolveMockFilePath(task, env, facts)
		if !ok {
			return mockUnsafePath("I could not confirm which file to rename.")
		}
		newName := dst
		if newName == "" {
			newName = filepath.Base(path)
		}
		return &AgentResponse{
			Script:      renameMockScript(path, newName),
			Explanation: fmt.Sprintf("Renames %s to %s.", path, newName),
			IsSafe:      true,
		}

	case mockDisk:
		return &AgentResponse{
			Script:      diskMockScript(),
			Explanation: "Shows used and free space for local filesystem drives.",
			IsSafe:      true,
		}

	case mockProcess:
		return &AgentResponse{
			Script:      processMockScript(),
			Explanation: "Shows the ten processes using the most CPU.",
			IsSafe:      true,
		}

	case mockFind:
		dir, ok := resolveMockDirectory(task, env, facts, true)
		if !ok {
			return mockUnsafePath("I could not confirm the folder to search.")
		}
		query := searchQuery(task)
		return &AgentResponse{
			Script:      findMockScript(dir, query),
			Explanation: fmt.Sprintf("Searches recursively under %s for names containing %q.", dir, query),
			IsSafe:      true,
		}

	default:
		dir, ok := resolveMockDirectory(task, env, facts, true)
		if !ok {
			return mockUnsafePath("I could not confirm the folder to list.")
		}
		filesOnly, dirsOnly := listFilters(task)
		return &AgentResponse{
			Script:      listMockScript(dir, filesOnly, dirsOnly),
			Explanation: fmt.Sprintf("Lists the contents of %s.", dir),
			IsSafe:      true,
		}
	}
}

func mockUnsafePath(reason string) *AgentResponse {
	return &AgentResponse{Explanation: reason, IsSafe: false}
}

func repairMockResponse(intent mockIntent, task string, env mockEnv, facts []mockFact, activeError string) *AgentResponse {
	lower := strings.ToLower(activeError)

	if strings.Contains(lower, "permissiondenied") || strings.Contains(lower, "permission denied") {
		dir, _ := resolveMockDirectory(task, env, facts, false)
		return &AgentResponse{
			Script:      listMockScript(dir, false, false),
			Explanation: "The previous attempt was blocked by permissions, so this retry uses a read-only listing instead.",
			IsSafe:      true,
		}
	}

	dir, ok := resolveMockDirectory(task, env, facts, false)
	if !ok {
		return nil
	}

	switch intent {
	case mockArchive:
		ext := requestedExtension(task)
		if ext == "" {
			ext = ".log"
		}
		return &AgentResponse{
			Script:      archiveFallbackMockScript(dir, ext),
			Explanation: "Compression failed, so this retry copies the matching files into a timestamped folder instead.",
			IsSafe:      true,
		}
	case mockDelete:
		ext := requestedExtension(task)
		if ext == "" {
			ext = ".tmp"
		}
		return &AgentResponse{
			Script:      deleteFallbackMockScript(dir, ext),
			Explanation: "The first removal failed, so this retry skips files that are locked or already gone.",
			IsSafe:      true,
		}
	}

	return &AgentResponse{
		Script:      listMockScript(dir, false, false),
		Explanation: "The previous approach failed, so this retry inspects the current state with a read-only listing.",
		IsSafe:      true,
	}
}

// ---------------------------------------------------------------------------
// PowerShell generators
// ---------------------------------------------------------------------------

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func listMockScript(dir string, filesOnly, dirsOnly bool) string {
	switch {
	case dirsOnly:
		return fmt.Sprintf(`Get-ChildItem -Path %s -Directory | Select-Object Name, LastWriteTime | Sort-Object Name | Format-Table -AutoSize`, psQuote(dir))
	case filesOnly:
		return fmt.Sprintf(`Get-ChildItem -Path %s -File | Select-Object Name, Length, LastWriteTime | Sort-Object Name | Format-Table -AutoSize`, psQuote(dir))
	default:
		return fmt.Sprintf(`Get-ChildItem -Path %s | Select-Object Name, @{Name='Type';Expression={if ($_.PSIsContainer) { 'Directory' } else { 'File' }}}, Length, LastWriteTime | Sort-Object Name | Format-Table -AutoSize`, psQuote(dir))
	}
}

func findMockScript(dir, query string) string {
	filter := "*"
	if query != "" && query != "*" {
		filter = "*" + query + "*"
	}
	return fmt.Sprintf(`Get-ChildItem -Path %s -Recurse -File -Filter %s -ErrorAction SilentlyContinue | Select-Object -First 20 FullName, Length | Format-Table -AutoSize`,
		psQuote(dir), psQuote(filter))
}

func archiveMockScript(dir, ext string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -Filter %s -File -ErrorAction Stop)
if ($files.Count -eq 0) { Write-Output "No %s files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would archive $($files.Count) %s file(s) from $source"; return }
$destination = Join-Path $source ('archive-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.zip')
Compress-Archive -Path $files.FullName -DestinationPath $destination -Force
Write-Output "Archived $($files.Count) file(s) to $destination"`,
		psQuote(dir), psQuote("*"+ext), ext, ext)
}

func deleteMockScript(dir, ext string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -Filter %s -File -ErrorAction Stop)
if ($files.Count -eq 0) { Write-Output "No %s files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would remove $($files.Count) %s file(s) from $source"; return }
$files | Remove-Item -Force -ErrorAction Stop
Write-Output "Removed $($files.Count) file(s) from $source"`,
		psQuote(dir), psQuote("*"+ext), ext, ext)
}

func createMockScript(base, name string) string {
	return fmt.Sprintf(`$path = Join-Path %s %s
if (Test-Path -LiteralPath $path) { Write-Output "Folder already exists: $path"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would create $path"; return }
New-Item -Path $path -ItemType Directory -Force | Out-Null
Write-Output "Created $path"`, psQuote(base), psQuote(name))
}

func countMockScript(dir string) string {
	return fmt.Sprintf(`$files = @(Get-ChildItem -Path %s -File)
Write-Output ("{0} file(s) in {1}" -f $files.Count, %s)
$files | Group-Object Extension | Sort-Object Count -Descending | Select-Object Count, Name | Format-Table -AutoSize`,
		psQuote(dir), psQuote(dir))
}

func largestMockScript(dir string) string {
	return fmt.Sprintf(`Get-ChildItem -Path %s -File | Sort-Object Length -Descending | Select-Object -First 10 Name, @{Name='SizeMB';Expression={[math]::Round($_.Length / 1MB, 2)}}, LastWriteTime | Format-Table -AutoSize`, psQuote(dir))
}

func readMockScript(path string) string {
	return fmt.Sprintf(`$path = %s
if (-not (Test-Path -LiteralPath $path)) { throw "File not found: $path" }
Get-Content -LiteralPath $path -TotalCount 50`, psQuote(path))
}

func openMockScript(path string) string {
	return fmt.Sprintf(`$path = %s
if (-not (Test-Path -LiteralPath $path)) { throw "Path not found: $path" }
if ($WhatIfPreference) { Write-Output "WhatIf: would open $path"; return }
Start-Process -FilePath $path
Write-Output "Opened $path"`, psQuote(path))
}

func moveMockScript(source, dest, ext string) string {
	filter := ""
	if ext != "" {
		filter = " -Filter " + psQuote("*"+ext)
	}
	return fmt.Sprintf(`$source = %s
$destination = %s
$files = @(Get-ChildItem -Path $source%s -File -ErrorAction Stop)
if ($files.Count -eq 0) { Write-Output "No matching files found in $source"; return }
if (-not (Test-Path -LiteralPath $destination)) { throw "Destination not found: $destination" }
if ($WhatIfPreference) { Write-Output "WhatIf: would move $($files.Count) file(s) to $destination"; return }
$files | Move-Item -Destination $destination -Force -ErrorAction Stop
Write-Output "Moved $($files.Count) file(s) to $destination"`,
		psQuote(source), psQuote(dest), filter)
}

func organizeMockScript(dir string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -File -ErrorAction Stop)
if ($files.Count -eq 0) { Write-Output "No files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would organize $($files.Count) file(s) in $source by extension"; return }
foreach ($file in $files) {
  $extension = $file.Extension.TrimStart('.').ToUpperInvariant()
  if ([string]::IsNullOrWhiteSpace($extension)) { $extension = 'NO_EXTENSION' }
  $folder = Join-Path $source $extension
  if (-not (Test-Path -LiteralPath $folder)) { New-Item -Path $folder -ItemType Directory | Out-Null }
  Move-Item -LiteralPath $file.FullName -Destination $folder -Force
}
Write-Output "Organized $($files.Count) file(s) by extension in $source"`, psQuote(dir))
}

func renameMockScript(path, newName string) string {
	return fmt.Sprintf(`$source = %s
$newName = %s
if (-not (Test-Path -LiteralPath $source)) { throw "File not found: $source" }
if ($WhatIfPreference) { Write-Output "WhatIf: would rename $source to $newName"; return }
Rename-Item -LiteralPath $source -NewName $newName -ErrorAction Stop
Write-Output "Renamed $source to $newName"`, psQuote(path), psQuote(newName))
}

func renameExtMockScript(dir, srcExt, dstExt string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -Filter %s -File -ErrorAction Stop)
if ($files.Count -eq 0) { Write-Output "No %s files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would rename $($files.Count) %s file(s) to %s"; return }
foreach ($file in $files) {
  $newName = [System.IO.Path]::ChangeExtension($file.Name, %s)
  Rename-Item -LiteralPath $file.FullName -NewName $newName -ErrorAction Stop
}
Write-Output "Renamed $($files.Count) file(s) to %s"`,
		psQuote(dir), psQuote("*"+srcExt), srcExt, srcExt, dstExt, psQuote(dstExt), dstExt)
}

func diskMockScript() string {
	return `Get-PSDrive -PSProvider FileSystem | Select-Object Name, @{Name='UsedGB';Expression={[math]::Round($_.Used / 1GB, 2)}}, @{Name='FreeGB';Expression={[math]::Round($_.Free / 1GB, 2)}} | Format-Table -AutoSize`
}

func processMockScript() string {
	return `Get-Process | Sort-Object CPU -Descending | Select-Object -First 10 Name, Id, @{Name='CPU';Expression={[math]::Round($_.CPU, 1)}}, @{Name='MemoryMB';Expression={[math]::Round($_.WorkingSet64 / 1MB, 1)}} | Format-Table -AutoSize`
}

func archiveFallbackMockScript(dir, ext string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -Filter %s -File -ErrorAction SilentlyContinue)
if ($files.Count -eq 0) { Write-Output "No %s files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would copy $($files.Count) file(s) to a timestamped folder"; return }
$destination = Join-Path $source ('archive-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -Path $destination -ItemType Directory -Force | Out-Null
$files | Copy-Item -Destination $destination -Force
Write-Output "Copied $($files.Count) file(s) to $destination because compression failed"`,
		psQuote(dir), psQuote("*"+ext), ext)
}

func deleteFallbackMockScript(dir, ext string) string {
	return fmt.Sprintf(`$source = %s
$files = @(Get-ChildItem -Path $source -Filter %s -File -ErrorAction SilentlyContinue)
if ($files.Count -eq 0) { Write-Output "No %s files found in $source"; return }
if ($WhatIfPreference) { Write-Output "WhatIf: would remove $($files.Count) %s file(s) from $source"; return }
foreach ($file in $files) { Remove-Item -LiteralPath $file.FullName -Force -ErrorAction SilentlyContinue }
Write-Output "Removed $($files.Count) file(s) from $source (skipping any that were locked)"`,
		psQuote(dir), psQuote("*"+ext), ext, ext)
}
