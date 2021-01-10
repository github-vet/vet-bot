package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"strings"
	"text/template"

	"github.com/github-vet/bots/internal/db"
	"github.com/google/go-github/v32/github"
)

// Md5Checksum represents an MD5 checksum as per the standard library.
type Md5Checksum [md5.Size]byte

// IssueReporter reports issues and maintains an in-memory store of reported code snippets to prevent
// exact duplicates from being reported.
type IssueReporter struct {
	bot   *VetBot
	md5s  map[Md5Checksum]struct{} // hashes of the code reported to protect against vendored / duplicated code
	owner string
	repo  string
}

// NewIssueReporter constructs a new issue reporter with the provided bot. The issue file will be
// created if it doesn't already exist. It stores a list of issues which have already been opened.
func NewIssueReporter(bot *VetBot, owner, repo string) (*IssueReporter, error) {
	md5s, err := readMd5sFromDB(bot)
	if err != nil {
		return nil, err
	}

	return &IssueReporter{
		bot:   bot,
		md5s:  md5s,
		owner: owner,
		repo:  repo,
	}, nil
}

func readMd5sFromDB(bot *VetBot) (map[Md5Checksum]struct{}, error) {
	sumSlices, err := db.FindingDAO.ListChecksums(context.Background(), bot.db)
	if err != nil {
		return nil, err
	}

	result := make(map[Md5Checksum]struct{}, len(sumSlices))
	var sum [16]byte
	for _, sumSlice := range sumSlices {
		copy(sum[:], sumSlice)
		result[sum] = struct{}{}
	}
	return result, nil
}

// ReportVetResult asynchronously creates a new GitHub issue to report the findings of the VetResult.
func (ir *IssueReporter) ReportVetResult(result VetResult) {
	md5Sum := md5.Sum([]byte(result.Quote))
	if _, ok := ir.md5s[md5Sum]; ok {
		log.Printf("found duplicated code in %s", result.FilePath)
		return
	}
	ir.md5s[md5Sum] = struct{}{}

	ir.bot.wg.Add(1)
	go func(result VetResult) {
		var (
			iss *github.Issue
			err error
		)
		if shouldReportToGithub(result.FilePath) {
			issueRequest := CreateIssueRequest(result)
			iss, _, err = ir.bot.client.CreateIssue(ir.owner, ir.repo, &issueRequest)
			if err != nil {
				log.Printf("error opening new issue: %v", err)
				return
			}
		}

		ir.persistResult(result, iss, md5Sum)
		ir.bot.wg.Done()
	}(result)
}

func shouldReportToGithub(filepath string) bool {
	if strings.HasSuffix(filepath, "_test.go") ||
		strings.Contains(filepath, "vendor/") ||
		strings.Contains(filepath, "test/") {
		return false
	}
	return true
}

// persistResult writes the provided VetResult and github.Issue to the database (if the issue is non-nil).
// It is not thread-safe (yet).
func (ir *IssueReporter) persistResult(result VetResult, issue *github.Issue, md5Sum [16]byte) error {
	createResult, err := db.FindingDAO.Create(context.Background(), ir.bot.db, db.Finding{
		GithubOwner:  result.Owner,
		GithubRepo:   result.Repo,
		Filepath:     result.FilePath,
		RootCommitID: result.RootCommitID,
		Quote:        result.Quote,
		QuoteMD5Sum:  db.Md5Sum(md5Sum[:]),
		StartLine:    result.Start.Line,
		EndLine:      result.End.Line,
		Message:      result.Message,
		ExtraInfo:    result.ExtraInfo,
	})
	if err != nil {
		return fmt.Errorf("error persisting finding: %w", err)
	}
	findingID, err := createResult.LastInsertId()
	if err != nil {
		return fmt.Errorf("error retrieving finding ID: %w", err)
	}

	if issue == nil {
		return nil
	}
	_, err = db.IssueDAO.Upsert(context.Background(), ir.bot.db, db.Issue{
		FindingID:   findingID,
		GithubOwner: ir.owner,
		GithubRepo:  ir.repo,
		GithubID:    issue.GetNumber(),
	})
	if err != nil {
		return fmt.Errorf("error persisting issue: %w", err)
	}
	log.Printf("opened new issue at %s", issue.GetHTMLURL())
	return nil
}

// CreateIssueRequest writes the header and description of the GitHub issue which is opened with the result
// of any findings.
func CreateIssueRequest(result VetResult) github.IssueRequest {

	slocCount := result.End.Line - result.Start.Line + 1
	title := fmt.Sprintf("%s/%s: %s; %d LoC", result.Owner, result.Repo, result.FilePath, slocCount)
	body := Description(result)
	labels := Labels(result)
	state := State(result)

	// TODO: labels based on lines of code
	return github.IssueRequest{
		Title:  &title,
		Body:   &body,
		Labels: &labels,
		State:  &state,
	}
}

// Description writes the description of an issue, given a VetResult.
func Description(result VetResult) string {
	permalink := result.Permalink()
	slocCount := result.End.Line - result.Start.Line + 1

	var b strings.Builder
	err := parsed.Execute(&b, IssueResult{
		VetResult: result,
		Link:      permalink,
		SlocCount: slocCount,
	})
	if err != nil {
		log.Printf("could not create description: %v", err)
	}
	return b.String()
}

// Labels returns the list of labels to be applied to a VetResult.
func Labels(result VetResult) []string {
	slocCount := result.End.Line - result.Start.Line
	labels := []string{"fresh"}
	if slocCount < 10 {
		labels = append(labels, "tiny")
	} else if slocCount < 50 {
		labels = append(labels, "small")
	} else if slocCount < 100 {
		labels = append(labels, "medium")
	} else if slocCount < 250 {
		labels = append(labels, "large")
	} else {
		labels = append(labels, "huge")
	}
	if strings.HasSuffix(result.FilePath, "_test.go") {
		labels = append(labels, "test")
	}
	if strings.HasPrefix(result.FilePath, "vendor/") {
		labels = append(labels, "vendored")
	}
	return labels
}

// State returns the desired status of a VetResult on issue creation.
func State(result VetResult) string {
	if strings.HasSuffix(result.FilePath, "_test.go") {
		return "closed"
	}
	return "open"
}

// IssueResult enriches a VetResult with some additional information.
type IssueResult struct {
	VetResult
	Link      string
	SlocCount int
}

// IssueResultTemplate is the template used to file a GitHub issue. It's meant to be invoked with an
// IssueResult.
// TODO: link to the README in the issues repository for more information.
var IssueResultTemplate string = `
Found a possible issue in [{{.Repository.Owner}}/{{.Repository.Repo}}](https://www.github.com/{{.Repository.Owner}}/{{.Repository.Repo}}) at [{{.FilePath}}]({{.Link}})

Below is the message reported by the analyzer for this snippet of code. Beware that the analyzer only reports the first issue it finds, so please do not limit your consideration to the contents of the below message.

> {{.Message}}

[Click here to see the code in its original context.]({{.Link}})

<details>
<summary>Click here to show the {{.SlocCount}} line(s) of Go which triggered the analyzer.</summary>

~~~go
{{.Quote}}
~~~
</details>

{{if .ExtraInfo}}
<details>
<summary>Click here to show extra information the analyzer produced.</summary>

~~~
{{.ExtraInfo}}
~~~
</details>
{{end}}

Leave a reaction on this issue to contribute to the project by classifying this instance as a **Bug** :-1:, **Mitigated** :+1:, or **Desirable Behavior** :rocket:
See the descriptions of the classifications [here](https://github.com/github-vet/rangeclosure-findings#how-can-i-help) for more information.

commit ID: {{.RootCommitID}}
`

var parsed *template.Template

func init() {
	IssueResultTemplate = strings.NewReplacer("~", "`").Replace(IssueResultTemplate)
	var err error
	parsed, err = template.New("issue").Parse(IssueResultTemplate)
	if err != nil {
		panic(err)
	}
}
