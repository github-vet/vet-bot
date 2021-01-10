package db

import (
	"context"
	"database/sql"

	"github.com/kalexmills/proteus"
)

type Gopher struct {
	Username          string `prof:"username"`
	DisagreementCount int    `prof:"disagreement_count"`
	AssessmentCount   int    `prof:"assessment_count"`
}

type GopherDAOImpl struct {
	Upsert         func(ctx context.Context, q proteus.ContextExecutor, g Gopher) (sql.Result, error)   `proq:"q:upsert" prop:"g"`
	FindByUsername func(ctx context.Context, q proteus.ContextQuerier, username string) (Gopher, error) `proq:"q:findByUsername" prop:"username"`
}

var GopherDAO GopherDAOImpl

func init() {
	m := proteus.MapMapper{
		"findByUsername": `SELECT * FROM gophers WHERE username = :username:`,

		"upsert": `INSERT INTO gophers (username, disagreement_count, assessment_count) 
									VALUES (:g.Username:, :g.DisagreementCount:, :g.AssessmentCount:)
							 ON CONFLICT(username) DO UPDATE
							 SET disagreement_count = :g.DisagreementCount:, 
									 assessment_count   = :g.AssessmentCount:`,
	}
	err := proteus.ShouldBuild(context.Background(), &GopherDAO, proteus.Sqlite, m)
	if err != nil {
		panic(err)
	}
}
