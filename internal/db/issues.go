package db

import (
	"context"
	"database/sql"

	"github.com/kalexmills/proteus"
)

type Issue struct {
	FindingID          int64  `prof:"finding_id"`
	GithubOwner        string `prof:"github_owner"`
	GithubRepo         string `prof:"github_repo"`
	GithubID           int    `prof:"github_id"`
	ExpertAssessment   string `prof:"expert_assessment"`
	ExpertDisagreement int    `prof:"expert_disagreement"`
}

func (i *Issue) ExpertsDisagree() bool {
	return i.ExpertDisagreement == 1
}

func (i *Issue) SetExpertsDisagree(value bool) {
	if value {
		i.ExpertDisagreement = 1
	} else {
		i.ExpertDisagreement = 0
	}
}

type IssueDAOImpl struct {
	FindByCoordinates func(ctx context.Context, q proteus.ContextQuerier, owner, repo string, githubID int) (Issue, error) `proq:"q:findByFindingID" prop:"owner,repo,githubID"`
	Upsert            func(ctx context.Context, q proteus.ContextExecutor, i Issue) (sql.Result, error)                    `proq:"q:upsert" prop:"i"`
}

var IssueDAO IssueDAOImpl

func init() {
	m := proteus.MapMapper{
		"findByFindingID": `SELECT * from issues 
												WHERE github_owner = :owner: AND
															github_repo = :repo: AND
															github_id = :githubID:`,

		"upsert": `INSERT INTO issues (finding_id, github_owner, github_repo, github_id, expert_assessment, expert_disagreement)
									VALUES (:i.FindingID:, :i.GithubOwner:, :i.GithubRepo:, :i.GithubID:, :i.ExpertAssessment:, :i.ExpertDisagreement:)
								ON CONFLICT (finding_id) DO UPDATE
									SET github_owner = :i.GithubOwner:,
											github_repo = :i.GithubRepo:,
											github_id = :i.GithubID:,
											expert_assessment = :i.ExpertAssessment:,
											expert_disagreement = :i.ExpertDisagreement:`,
	}
	err := proteus.ShouldBuild(context.Background(), &IssueDAO, proteus.Sqlite, m)
	if err != nil {
		panic(err)
	}
}
