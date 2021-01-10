package db

import (
	"context"
	"database/sql"

	"github.com/kalexmills/proteus"
)

type Repository struct {
	GithubOwner string    `prof:"github_owner"`
	GithubRepo  string    `prof:"github_repo"`
	State       RepoState `prof:"state"`
}

func NewRepository(owner, repo string) Repository {
	return Repository{owner, repo, RepoStateFresh}
}

type RepoState string

const (
	RepoStateFresh   RepoState = "F"
	RepoStateVisited RepoState = "V"
	RepoStateError   RepoState = "E"
)

func stringToRepoState(str string) RepoState {
	return RepoState(str[0])
}

type RepositoryDaoImpl struct {
	FindByID    func(ctx context.Context, q proteus.ContextQuerier, owner, repo string) (Repository, error) `proq:"q:findByID" prop:"owner,repo"`
	ListByState func(ctx context.Context, q proteus.ContextQuerier, state RepoState) ([]Repository, error)  `proq:"q:listByState" prop:"state"`
	Upsert      func(ctx context.Context, e proteus.ContextExecutor, r Repository) (sql.Result, error)      `proq:"q:upsert" prop:"r"`
	CountAll    func(ctx context.Context, q proteus.ContextQuerier) (int64, error)                          `proq:"q:count"`
}

var RepositoryDAO = RepositoryDaoImpl{}

func init() {
	m := proteus.MapMapper{
		"findByID":    "SELECT * FROM repositories WHERE github_owner = :owner: AND github_repo = :repo:",
		"listByState": "SELECT * from repositories WHERE state = :state:",
		"upsert": `INSERT INTO repositories (github_owner, github_repo, state) 
									VALUES (:r.GithubOwner:, :r.GithubRepo:, :r.State:)
								ON CONFLICT (github_owner, github_repo) DO UPDATE
								SET state = :r.State:`,
		"count": `SELECT count(*) FROM repositories`,
	}
	err := proteus.ShouldBuild(context.Background(), &RepositoryDAO, proteus.Sqlite, m)
	if err != nil {
		panic(err)
	}
}
