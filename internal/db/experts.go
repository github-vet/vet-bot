package db

import (
	"context"
	"database/sql"

	"github.com/kalexmills/proteus"
)

// Expert is a GitHub user marked as an expert
type Expert struct {
	Username        string `prof:"username"`
	AssessmentCount int    `prof:"assessment_count"`
}

type ExpertDAOImpl struct {
	Upsert         func(ctx context.Context, q proteus.ContextExecutor, e Expert) (sql.Result, error)   `proq:"q:upsert" prop:"e"`
	FindByUsername func(ctx context.Context, q proteus.ContextQuerier, username string) (Expert, error) `proq:"q:findByUsername" prop:"username"`
}

var ExpertDAO ExpertDAOImpl

func init() {
	m := proteus.MapMapper{
		"findByUsername": `SELECT * FROM experts WHERE username = :username:`,

		"upsert": `INSERT INTO experts (username, assessment_count) 
									VALUES (:e.Username:, :e.AssessmentCount:)
							 ON CONFLICT(username) DO UPDATE
							 SET assessment_count   = :e.AssessmentCount:`,
	}
	err := proteus.ShouldBuild(context.Background(), &ExpertDAO, proteus.Sqlite, m)
	if err != nil {
		panic(err)
	}
}
